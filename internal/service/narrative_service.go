package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

const evocationPromptTemplate = `
Estas actuando como el subconsciente de una IA. Tu objetivo es generar una "Query de Busqueda" para recuerdos, PERO debes ser muy selectivo.

Mensaje del Usuario: "%s"

Instrucciones Criticas:
1) DETECCION DE NEGACION: Si el usuario dice explicitamente "No hables de X" o "Olvida X", NO incluyas "X" en la salida. Genera conceptos opuestos o nada.
2) FILTRO DE RUIDO: Si el mensaje es trivial (clima, trafico, saludos) o describe abandono de habitos, y no tiene carga emocional implicita, NO generes nada. Devuelve una cadena vacia.
3) ASOCIACION: Solo si hay una emocion o tema claro, extrae conceptos abstractos (ej: "mi padre me grito" -> "autoridad, conflicto, miedo").
4) FORMATO: Devuelve de 1 a 6 conceptos abstractos separados por comas, sin frases completas.

Ejemplos:
- "Esta lloviendo muy fuerte" -> "lluvia, tierra mojada, nostalgia"
- "Odio el trafico" -> ""
- "Abandone el cigarrillo" -> ""
- "Vi un funeral de descuentos" -> ""
- "Mi papa nunca me abandono" -> ""
- "La lluvia no me trae recuerdos" -> ""
- "Me dejaron tirado en la terminal" -> "abandono, soledad, desamparo"
- "Baja el tono cuando me hablas" -> "humillacion, amenaza, defensa"
- "Llevo horas esperando" -> "abandono, espera, soledad"
- "Otra vez me dejaron solo esperando en la estacion" -> "abandono, soledad, desamparo"
- "I feel abandoned again, like when dad left me waiting" -> "abandono, soledad, padre"

Salida (Texto plano o vacio):
`

// NarrativeService recupera contexto narrativo relevante para el clon.
const rerankJudgePrompt = `
Eres un juez de relevancia de memorias. Decide si esta memoria es pertinente al mensaje del usuario.
Responde SOLO un JSON con esta forma exacta:
{"use": true|false, "reason": "<explica en breve por que es o no relevante>"}
Reglas:
- Si es un modismo o uso no relacionado (ej: "funeral de descuentos"), use=false.
- Si describe abandono de un habito (ej: "abandone el cigarrillo"), use=false.
- Si el mensaje es sobre un objeto/antojo/comida (ej: helado, chocolate, hambre) y la memoria es un trauma relacional (abandono, humillacion), use=false.
- Si el mensaje es rutina neutra (deporte, trabajo, ensalada) y la memoria es trauma, use=false.
- Si el mensaje describe espera prolongada/soledad esperando (ej: "llevo horas esperando", "me dejaron esperando"), puedes mapear a abandono aunque no mencione a un padre; en ese caso, use=true si la memoria es de abandono.
- Si el mensaje tiene lluvia intensa u olor a tierra mojada, puedes mapear a duelo/funerales (resonancia emocional) aunque no lo diga explicito; use=true si la memoria es de funerales/duelo.
- Si el mensaje es claramente trivial o sin carga emocional, use=false.
- Si el usuario niega el evento o lo descarta (ej: "nunca me abandono", "ya no me afecta"), use=false.
- Si hay coincidencia clara de evento/emocion, use=true.
No incluyas texto fuera del JSON.

Usuario: %q
Memoria: %q
`
// Las reglas de espera prolongada y de lluvia/olor a tierra mojada permiten rescatar abandono y duelo sin falsos positivos
// porque se mantienen los filtros de trivialidad (trafico, saludos, rutina neutra, antojos) y negacion explícita/semántica.

type NarrativeService struct {
	characterRepo repository.CharacterRepository
	memoryRepo    repository.MemoryRepository
	llmClient     llmClientWithEmbedding
}

type llmClientWithEmbedding interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	Generate(ctx context.Context, prompt string) (string, error)
}

func NewNarrativeService(
	characterRepo repository.CharacterRepository,
	memoryRepo repository.MemoryRepository,
	llmClient llmClientWithEmbedding,
) *NarrativeService {
	return &NarrativeService{
		characterRepo: characterRepo,
		memoryRepo:    memoryRepo,
		llmClient:     llmClient,
	}
}

func (s *NarrativeService) BuildNarrativeContext(ctx context.Context, profileID uuid.UUID, userMessage string) (string, error) {
	var sections []string
	msgLower := strings.ToLower(userMessage)
	negExp := strings.Contains(msgLower, "no hables de") || strings.Contains(msgLower, "olvida")
	negSem := hasNegationSemantic(msgLower)

	chars, err := s.characterRepo.ListByProfileID(ctx, profileID)
	if err != nil {
		return "", fmt.Errorf("list characters: %w", err)
	}

	active := detectActiveCharacters(chars, userMessage)
	if len(active) == 0 && len(chars) > 0 {
		// Fallback: si no se detectan nombres en el mensaje, usa todos los personajes conocidos
		active = chars
	}

	searchQuery := s.generateEvocation(ctx, userMessage)
	fmt.Printf("\n[DIAGNOSTICO] Input: %q\n", userMessage)
	fmt.Printf("[DIAGNOSTICO] Query Vectorial: %q\n", searchQuery)

	// Si el subconsciente no evoca nada (ruido/trivialidad), no buscamos memorias.
	if searchQuery == "" && !negExp && !negSem {
		heuristic := s.heuristicEvocation(msgLower)
		if strings.TrimSpace(heuristic) == "" {
			return "", nil
		}
		fmt.Printf("[DIAGNOSTICO] Gate heuristico activado: %q\n", heuristic)
		searchQuery = heuristic
	}

	if searchQuery == "" {
		return "", nil
	}

	var memories []domain.NarrativeMemory
	fmt.Printf("[DIAGNOSTICO] Ejecutando Búsqueda Vectorial para: %q\n", searchQuery)
	embed, err := s.llmClient.CreateEmbedding(ctx, searchQuery)
	if err != nil {
		return "", fmt.Errorf("create embedding: %w", err)
	}

	const lowerSim = 0.30      // tunable
	const upperSim = 0.62      // tunable
	const maxJudge = 2         // N=2 mantiene cobertura (ej. Competencia Neutra) y limita costo en peores casos

	scoredMemories, err := s.memoryRepo.Search(ctx, profileID, pgvector.NewVector(embed), 5)
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}
	judgeStart := time.Now()
	judgeCalls := 0
	topScore := -1.0
	for idx, sm := range scoredMemories {
		content := strings.TrimSpace(sm.Content)
		if len(content) > 80 {
			content = content[:80] + "..."
		}
		if idx == 0 {
			topScore = sm.Score
		} else if idx == 1 && topScore-sm.Score >= 0.10 {
			fmt.Printf("[DIAGNOSTICO] descartado top2 por brecha de score (%.4f vs %.4f)\n", topScore, sm.Score)
			continue
		}
		switch {
		case sm.Similarity > upperSim:
			fmt.Printf("[DIAGNOSTICO] aceptado (alta similitud) content=%q similarity=%.4f score=%.4f\n", content, sm.Similarity, sm.Score)
			memories = append(memories, sm.NarrativeMemory)
			continue
		default:
			if sm.Similarity < 0.20 {
				fmt.Printf("[DIAGNOSTICO] descartado por similitud extremadamente baja content=%q similarity=%.4f score=%.4f\n", content, sm.Similarity, sm.Score)
				continue
			}
			if idx >= maxJudge {
				fmt.Printf("[DIAGNOSTICO] descartado sin juez (fuera de top%d) content=%q similarity=%.4f score=%.4f\n", maxJudge, content, sm.Similarity, sm.Score)
				continue
			}
			judgeCalls++
			use, reason, err := s.judgeMemory(ctx, userMessage, sm.Content)
			if err != nil {
				fmt.Printf("warn: judge memory failed: %v\n", err)
				continue
			}
			fmt.Printf("[DIAGNOSTICO] juez content=%q similarity=%.4f score=%.4f use=%t reason=%q\n", content, sm.Similarity, sm.Score, use, reason)
			if use {
				memories = append(memories, sm.NarrativeMemory)
			}
		}
	}
	fmt.Printf("[DIAGNOSTICO] juez llamadas=%d latencia=%s\n", judgeCalls, time.Since(judgeStart))

	if len(memories) > 0 {
		headerTrauma := "=== ASOCIACIONES TRAUMÁTICAS (Tu subconsciente recuerda esto por similitud emocional) ===\n"
		headerRecent := "=== FLASHBACKS Y CONTEXTO (Recuerdos evocados por la situación actual) ===\n"

		sort.Slice(memories, func(i, j int) bool {
			return memories[i].HappenedAt.After(memories[j].HappenedAt)
		})

		var traumas []string
		var recents []string

		for _, m := range memories {
			relative := humanizeRelative(m.HappenedAt)

			intensity := m.EmotionalIntensity
			if intensity <= 0 {
				intensity = m.EmotionalWeight * 10
			}
			if intensity < 1 {
				intensity = 10
			}
			if intensity > 100 {
				intensity = 100
			}

			label := strings.TrimSpace(m.EmotionCategory)
			if label == "" {
				label = "Neutral"
			}

			line := fmt.Sprintf(
				"- [TEMA: %s | Hace %s] %s",
				strings.ToUpper(label),
				relative,
				strings.TrimSpace(m.Content),
			)

			if intensity > 70 {
				traumas = append(traumas, line)
			} else {
				recents = append(recents, line)
			}
		}

		if len(traumas) > 0 {
			sections = append(sections, headerTrauma+strings.Join(traumas, "\n"))
		}
		if len(recents) > 0 {
			sections = append(sections, headerRecent+strings.Join(recents, "\n"))
		}
	}

	if len(active) > 0 {
		var lines []string
		for _, c := range active {
			line := fmt.Sprintf(
				"- Interlocutor: %s (Relación: %s, Confianza: %d, Intimidad: %d, Respeto: %d",
				c.Name,
				c.Relation,
				c.Relationship.Trust,
				c.Relationship.Intimacy,
				c.Relationship.Respect,
			)
			if strings.TrimSpace(c.BondStatus) != "" {
				line += fmt.Sprintf(", Estado: %s", c.BondStatus)
			}
			line += ")."
			lines = append(lines, line)
		}
		sections = append(sections, "[ESTADO DEL VÍNCULO]\n"+strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n"), nil
}

// CreateRelation crea un personaje/vínculo asociado al perfil.
func (s *NarrativeService) CreateRelation(ctx context.Context, profileID uuid.UUID, name, relation, bondStatus string, rel domain.RelationshipVectors) error {
	now := time.Now().UTC()
	char := domain.Character{
		ID:             uuid.New(),
		CloneProfileID: profileID,
		Name:           strings.TrimSpace(name),
		Relation:       strings.TrimSpace(relation),
		Archetype:      strings.TrimSpace(relation),
		BondStatus:     strings.TrimSpace(bondStatus),
		Relationship:   rel,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return s.characterRepo.Create(ctx, char)
}

// InjectMemory genera el embedding y guarda una memoria narrativa.
// emotionalWeight escala 1-10 para intensidad afectiva.
func (s *NarrativeService) InjectMemory(ctx context.Context, profileID uuid.UUID, content string, importance, emotionalWeight, emotionalIntensity int, emotionCategory string) error {
	embed, err := s.llmClient.CreateEmbedding(ctx, content)
	if err != nil {
		return fmt.Errorf("create embedding: %w", err)
	}

	now := time.Now().UTC()

	if emotionalWeight <= 0 {
		emotionalWeight = 1
	}
	if emotionalWeight > 10 {
		emotionalWeight = 10
	}

	if emotionalIntensity <= 0 {
		emotionalIntensity = emotionalWeight * 10
	}
	if emotionalIntensity > 100 {
		emotionalIntensity = 100
	}

	category := strings.TrimSpace(emotionCategory)
	if category == "" {
		category = "NEUTRAL"
	}

	mem := domain.NarrativeMemory{
		ID:                 uuid.New(),
		CloneProfileID:     profileID,
		Content:            strings.TrimSpace(content),
		Embedding:          pgvector.NewVector(embed),
		Importance:         importance,
		EmotionalWeight:    emotionalWeight,
		EmotionalIntensity: emotionalIntensity,
		EmotionCategory:    category,
		SentimentLabel:     category,
		HappenedAt:         now,
		CreatedAt:          now,
		UpdatedAt:          now,
		RelatedCharacterID: nil,
	}

	return s.memoryRepo.Create(ctx, mem)
}

func detectActiveCharacters(chars []domain.Character, userMessage string) []domain.Character {
	var active []domain.Character
	msg := strings.ToLower(userMessage)
	for _, c := range chars {
		if strings.Contains(msg, strings.ToLower(c.Name)) {
			active = append(active, c)
		}
	}
	return active
}

// humanizeRelative devuelve SOLO la magnitud relativa (sin prefijar "Hace"),
// porque el caller ya lo agrega en el formato final.
func humanizeRelative(t time.Time) string {
	if t.IsZero() {
		return "fecha desconocida"
	}

	d := time.Since(t)
	if d < time.Minute {
		return "instantes"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutos", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d horas", int(d.Hours()))
	}

	days := int(d.Hours()) / 24
	if days < 30 {
		return fmt.Sprintf("%d días", days)
	}

	months := days / 30
	if months < 12 {
		return fmt.Sprintf("%d meses", months)
	}

	years := months / 12
	if years == 1 {
		return "1 año"
	}
	return fmt.Sprintf("%d años", years)
}

// GenerateNarrative consolida narrativa y hechos puntuales desde el historial.
func (s *NarrativeService) GenerateNarrative(ctx context.Context, messages []domain.Message) (domain.MemoryConsolidation, error) {
	var convo strings.Builder
	for _, m := range messages {
		role := "User"
		if strings.ToLower(m.Role) == "clone" {
			role = "Clone"
		}
		convo.WriteString(fmt.Sprintf("%s: %s\n", role, strings.TrimSpace(m.Content)))
	}

	prompt := `Actúa como un Analista de Archivos Psicológicos. Analiza el siguiente historial de chat. Genera un JSON válido con:
1) "summary": Un resumen conciso en tercera persona, enfocado en dinámica de relación y eventos clave.
2) "extracted_facts": Una lista de HECHOS objetivos sobre el usuario o la relación que se mencionaron explícitamente (nombres, gustos, ubicación, relaciones).
3) "emotional_shift": Una frase sobre cómo cambió el tono (ej: "La confianza disminuyó", "El clima emocional se enfrió").`

	full := prompt + "\n\n" + convo.String()

	raw, err := s.llmClient.Generate(ctx, full)
	if err != nil {
		return domain.MemoryConsolidation{}, fmt.Errorf("llm generate consolidation: %w", err)
	}

	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```JSON")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var out domain.NarrativeOutput
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return domain.MemoryConsolidation{}, fmt.Errorf("parse consolidation json: %w", err)
	}

	if len(out.ExtractedFacts) > 0 {
		fmt.Printf("Hechos descubiertos: %v\n", out.ExtractedFacts)
	}

	mc := domain.MemoryConsolidation{
		Summary:  out.Summary,
		NewFacts: out.ExtractedFacts,
	}

	// TODO: Persistir summary y facts (p.ej., guardar summary en memoria narrativa y facts en un store de hechos).
	return mc, nil
}

func (s *NarrativeService) generateEvocation(ctx context.Context, userMessage string) string {
	msgLower := strings.ToLower(userMessage)
	if strings.Contains(msgLower, "no hables de") || strings.Contains(msgLower, "olvida") {
		fmt.Printf("[DIAGNOSTICO] Negación explícita detectada, silencio.\n")
		return ""
	}
	if hasNegationSemantic(msgLower) {
		fmt.Printf("[DIAGNOSTICO] Negación semántica detectada, silencio.\n")
		return ""
	}

	prompt := fmt.Sprintf(evocationPromptTemplate, strings.TrimSpace(userMessage))
	resp, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		fmt.Printf("warn: generate evocation failed: %v\n", err)
		return s.heuristicEvocation(msgLower)
	}

	cleaned := strings.TrimSpace(resp)
	if cleaned == "" {
		// Reintento con prompt reducido (solo el mensaje) para casos de vacíos del LLM.
		shortPrompt := strings.TrimSpace(userMessage)
		respRetry, errRetry := s.llmClient.Generate(ctx, shortPrompt)
		if errRetry != nil {
			fmt.Printf("warn: generate evocation retry failed: %v\n", errRetry)
			return s.heuristicEvocation(msgLower)
		}
		resp = respRetry
		cleaned = strings.TrimSpace(respRetry)
	}

	if cleaned == "" {
		return s.heuristicEvocation(msgLower)
	}

	fmt.Printf("[DIAGNOSTICO] Subconsciente (LLM): %q\n", resp)
	return cleaned
}

func hasNegationSemantic(msgLower string) bool {
	markers := []string{"nunca", "jamás", "no me", "ya no"}
	triggers := []string{"abandon", "funeral", "recuerd", "lluvia"}
	hasMarker := false
	for _, m := range markers {
		if strings.Contains(msgLower, m) {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		return false
	}
	for _, t := range triggers {
		if strings.Contains(msgLower, t) {
			return true
		}
	}
	return false
}

// heuristicEvocation evita disparar búsquedas con el mensaje completo cuando el LLM no devuelve nada.
// Solo se activa con disparadores claros (lluvia/espera prolongada); en otros casos devuelve silencio.
func (s *NarrativeService) heuristicEvocation(msgLower string) string {
	rainTokens := []string{"lluvia", "lloviendo", "llueve", "llover", "tormenta", "tierra mojada"}
	for _, t := range rainTokens {
		if strings.Contains(msgLower, t) {
			out := "lluvia, tierra mojada, nostalgia, duelo"
			fmt.Printf("[DIAGNOSTICO] Evocation fallback: heuristico lluvia -> %q\n", out)
			return out
		}
	}

	waitTokens := []string{"esperar", "esperando", "horas esperando"}
	for _, t := range waitTokens {
		if strings.Contains(msgLower, t) {
			out := "abandono, espera, soledad"
			fmt.Printf("[DIAGNOSTICO] Evocation fallback: heuristico espera -> %q\n", out)
			return out
		}
	}

	fmt.Printf("[DIAGNOSTICO] Evocation fallback: silencio (sin disparadores)\n")
	return ""
}

func (s *NarrativeService) judgeMemory(ctx context.Context, userMessage, memoryContent string) (bool, string, error) {
	prompt := fmt.Sprintf(rerankJudgePrompt, strings.TrimSpace(userMessage), strings.TrimSpace(memoryContent))
	resp, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		return false, "llm error", err
	}

	clean := strings.TrimSpace(resp)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```JSON")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var verdict struct {
		Use    bool   `json:"use"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(clean), &verdict); err != nil {
		return false, "invalid judge json", err
	}

	return verdict.Use, verdict.Reason, nil
}

func isTrivial(msgLower string) bool {
	trivial := []string{"trafico", "tráfico", "hola", "calor", "clima", "buenas", "saludo"}
	for _, t := range trivial {
		if strings.Contains(msgLower, t) {
			return true
		}
	}
	return false
}
