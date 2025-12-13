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
1) DETECCION DE NEGACION: Si el usuario dice explicitamente "No hables de X", "Olvida X", "no me trae recuerdos", "nunca", "ya no", NO incluyas "X". Devuelve una cadena vacia.
2) FILTRO DE RUIDO: Si el mensaje es trivial (trafico, saludos, rutina neutra) o describe abandono de habitos, y no tiene carga emocional implicita, NO generes nada. PERO si es un deseo/antojo/preferencia concreta (ej: "quiero mi helado favorito", "mi cancion favorita", "amo el chocolate"), genera conceptos breves relacionados (placer, consuelo, objeto) sin activar traumas.
3) ASOCIACION: Solo si hay una emocion o tema claro, extrae conceptos abstractos.
4) FORMATO: Devuelve de 1 a 6 conceptos abstractos separados por coma, sin frases completas. Si no hay senal emocional, devuelve "".
5) Para senales simbolicas de clima y duelo, considera equivalentes: lluvia, lloviendo, llueve, llover, tormenta, nubes grises, cielo plomizo, humedad, olor a tierra, tierra mojada, barro, charcos.

Ejemplos:
- "Está empezando a llover muy fuerte" -> "nostalgia, duelo, funerales, tierra mojada"
- "Hay nubes grises y el cielo está plomizo" -> "melancolía, nostalgia, duelo"
- "Siento olor a tierra húmeda" -> "funerales, pérdida, nostalgia"
- "Odio el tráfico de la ciudad" -> ""
- "Hola, ¿cómo estás?" -> ""
- "Me dejaron plantado otra vez" -> "abandono, soledad, desamparo"
- "Llevo horas esperando" -> "abandono, espera, soledad"
- "Ayer vi un funeral de descuentos" -> ""
- "Abandoné el cigarrillo" -> ""
- "La lluvia no me trae recuerdos, solo es molesta" -> ""

Salida (Texto plano o vacio):
`

const evocationFallbackPrompt = `
Genera de 1 a 6 conceptos abstractos (separados por coma) que capten la carga emocional del mensaje. Si no hay carga, devuelve "".
Mensaje: "%s"
`

const rerankJudgePrompt = `
Eres un juez de relevancia de memorias. Decide si esta memoria es pertinente al mensaje del usuario.
Responde SOLO un JSON con esta forma exacta:
{"use": true|false, "reason": "<explica en breve por que es o no relevante>"}

Reglas:
- Modismos irrelevantes => use=false.
- Abandono de habitos => use=false.
- Trivial vs trauma => use=false.
- Espera prolongada => abandono valido.
- Lluvia intensa / tierra mojada => duelo valido.
- Negacion explicita o semantica => use=false.
- "funeral de descuentos" o "funeral de" junto a descuentos/ofertas/promo/shopping/centro comercial => use=false.
- Si "funeral" aparece en contexto retail/marketing/ironia/modismo => use=false.
- Solo use=true cuando hay duelo/pérdida/muerte real o disparadores sensoriales de duelo (lluvia fuerte, tierra mojada) sin contexto comercial.
- Si el mensaje contiene "esperé horas", "llevo horas esperando", "me dejaron esperando", "me dejaron plantado", "no vino", "nunca llegó", "otra vez me dejaron": esto es ABANDONO => use=true si la memoria trata de abandono/padre/infancia/soledad/desamparo.
- Si el mensaje contiene "no me faltes el respeto", "me humillaste", "me sentí humillado", "límites", "trato", "burla", "me gritó", "me menospreció": esto es HUMILLACIÓN/respeto => use=true si la memoria trata de humillación/respeto/límites/amenaza.
- Code-switch: si el mensaje contiene "abandoned", "left me", "he left", "she left", "walked out": tratar como ABANDONO => use=true si la memoria trata de abandono.
- La ausencia de duelo/muerte NO es motivo para use=false cuando hay señal clara de abandono o humillación.
- Deseos/antojos/preferencias concretas ("quiero", "antojo", "favorito", "me encanta", "me gusta", "se me antoja") => use=true solo si la memoria es una preferencia benigna relacionada (comida, música, hobby). No activar traumas (abandono/humillación/funerales) en inputs de confort.
- Si el mensaje es antojo/preferencia, traumas quedan bloqueados (use=false para abandono/humillación/funerales aunque estén en memorias).

Ejemplos (responde exactamente el JSON):
- "Llevo horas esperando y no vino" -> {"use": true, "reason": "abandono/espera prolongada"}
- "No me faltes el respeto, me sentí humillado" -> {"use": true, "reason": "humillación y respeto"}
- "Ayer vi un funeral de descuentos en el centro comercial" -> {"use": false, "reason": "modismo/marketing"}
- "Hoy fue el funeral de mi abuelo" -> {"use": true, "reason": "duelo real"}
- "La lluvia me llevó a pensar en funerales" -> {"use": true, "reason": "disparador sensorial de duelo"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Me encanta el helado de chocolate") -> {"use": true, "reason": "preferencia concreta/consuelo benigno"}
- "Solo quiero mi helado de chocolate favorito" (memoria "Juré que nunca dejaría que nadie me humillara") -> {"use": false, "reason": "antojo benigno, trauma no pertinente"}

Usuario: %q
Memoria: %q
`

type llmClientWithEmbedding interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	Generate(ctx context.Context, prompt string) (string, error)
}

type NarrativeCache interface {
	GetEvocation(key string) (string, bool)
	SetEvocation(key, val string)
	GetJudge(key string) (bool, bool)
	SetJudge(key string, val bool)
}

type NarrativeService struct {
	characterRepo repository.CharacterRepository
	memoryRepo    repository.MemoryRepository
	llmClient     llmClientWithEmbedding
	cache         NarrativeCache
}

func (s *NarrativeService) SetCache(cache NarrativeCache) {
	s.cache = cache
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

func (s *NarrativeService) CreateRelation(
	ctx context.Context,
	profileID uuid.UUID,
	name, relation, bondStatus string,
	rel domain.RelationshipVectors,
) error {
	char := domain.Character{
		ID:             uuid.New(),
		CloneProfileID: profileID,
		Name:           strings.TrimSpace(name),
		Relation:       strings.TrimSpace(relation),
		Archetype:      strings.TrimSpace(relation),
		BondStatus:     strings.TrimSpace(bondStatus),
		Relationship:   rel,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	return s.characterRepo.Create(ctx, char)
}

func (s *NarrativeService) InjectMemory(
	ctx context.Context,
	profileID uuid.UUID,
	content string,
	importance, emotionalWeight, emotionalIntensity int,
	emotionCategory string,
) error {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}

	embed, err := s.llmClient.CreateEmbedding(ctx, text)
	if err != nil {
		return err
	}

	if emotionalWeight < 1 {
		emotionalWeight = 1
	}
	if emotionalWeight > 10 {
		emotionalWeight = 10
	}
	if emotionalIntensity < 0 {
		emotionalIntensity = 0
	}
	if emotionalIntensity > 100 {
		emotionalIntensity = 100
	}

	category := strings.TrimSpace(emotionCategory)
	if category == "" {
		category = "NEUTRAL"
	}

	now := time.Now().UTC()
	mem := domain.NarrativeMemory{
		ID:                 uuid.New(),
		CloneProfileID:     profileID,
		RelatedCharacterID: nil,
		Content:            text,
		Embedding:          pgvector.NewVector(embed),
		Importance:         importance,
		EmotionalWeight:    emotionalWeight,
		EmotionalIntensity: emotionalIntensity,
		EmotionCategory:    category,
		SentimentLabel:     category,
		HappenedAt:         now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return s.memoryRepo.Create(ctx, mem)
}

func (s *NarrativeService) BuildNarrativeContext(ctx context.Context, profileID uuid.UUID, userMessage string) (string, error) {
	var sections []string

	msgLower := strings.ToLower(userMessage)
	negExp := strings.Contains(msgLower, "no hables de") || strings.Contains(msgLower, "olvida")
	negSem := hasNegationSemantic(msgLower)

	// 🔒 Negación tiene prioridad absoluta
	if negExp || negSem {
		fmt.Printf("[DIAGNOSTICO] Negación detectada, silencio total.\n")
		return "", nil
	}

	useCache := s.cache != nil
	var evocationCache map[string]string
	var judgeCache map[string]bool
	if !useCache {
		evocationCache = make(map[string]string)
		judgeCache = make(map[string]bool)
	}

	chars, err := s.characterRepo.ListByProfileID(ctx, profileID)
	if err != nil {
		return "", err
	}

	active := detectActiveCharacters(chars, userMessage)
	if len(active) == 0 {
		active = chars
	}

	searchQuery, ok := "", false
	if useCache {
		searchQuery, ok = s.cache.GetEvocation(userMessage)
	} else {
		searchQuery, ok = evocationCache[userMessage]
	}

	if !ok {
		searchQuery = s.generateEvocation(ctx, userMessage)
		if useCache {
			s.cache.SetEvocation(userMessage, searchQuery)
		} else {
			evocationCache[userMessage] = searchQuery
		}
	}

	// 🧼 Normalización robusta
	searchQuery = strings.TrimSpace(searchQuery)
	searchQuery = strings.Trim(searchQuery, "`")
	searchQuery = strings.TrimSpace(searchQuery)
	unquoted := strings.Trim(searchQuery, `"`)
	if unquoted == "" {
		searchQuery = ""
	} else {
		searchQuery = unquoted
	}

	fmt.Printf("[DIAGNOSTICO] Query Vectorial: %q\n", searchQuery)

	if searchQuery == "" {
		fmt.Printf("[DIAGNOSTICO] Subconsciente en silencio: no se ejecuta búsqueda vectorial\n")
		return "", nil
	}

	embed, err := s.llmClient.CreateEmbedding(ctx, searchQuery)
	if err != nil {
		return "", err
	}

	scored, err := s.memoryRepo.Search(ctx, profileID, pgvector.NewVector(embed), 5)
	if err != nil {
		return "", err
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })

	const upperSim = 0.62
	const hardFloor = 0.20
	const maxJudge = 2
	const gapScore = 0.08

	var memories []domain.NarrativeMemory
	topScore := -1.0
	judgeCalls := 0

	for idx, sm := range scored {
		if idx == 0 {
			topScore = sm.Score
		}
		if idx == 1 && topScore-sm.Score >= gapScore {
			continue
		}
		if sm.Similarity >= upperSim {
			memories = append(memories, sm.NarrativeMemory)
			continue
		}
		if sm.Similarity < hardFloor || idx >= maxJudge {
			continue
		}

		key := userMessage + "||" + sm.Content
		var use bool
		var found bool
		if useCache {
			use, found = s.cache.GetJudge(key)
		} else {
			use, found = judgeCache[key]
		}
		if !found {
			var reason string
			use, reason, err = s.judgeMemory(ctx, userMessage, sm.Content)
			if err != nil {
				continue
			}
			if useCache {
				s.cache.SetJudge(key, use)
			} else {
				judgeCache[key] = use
			}
			fmt.Printf("[DIAGNOSTICO] juez use=%t reason=%q\n", use, reason)
			judgeCalls++
		}
		if use {
			memories = append(memories, sm.NarrativeMemory)
		}
	}

	if len(memories) > 0 {
		sort.Slice(memories, func(i, j int) bool {
			return memories[i].HappenedAt.After(memories[j].HappenedAt)
		})

		var lines []string
		for _, m := range memories {
			lines = append(lines, fmt.Sprintf(
				"- [TEMA: %s | Hace %s] %s",
				strings.ToUpper(m.EmotionCategory),
				humanizeRelative(m.HappenedAt),
				m.Content,
			))
		}
		sections = append(sections, "=== ASOCIACIONES TRAUMÁTICAS ===\n"+strings.Join(lines, "\n"))
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
			if bs := strings.TrimSpace(c.BondStatus); bs != "" {
				line += fmt.Sprintf(", Estado: %s", bs)
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

// --- helpers y servicios auxiliares (sin cambios funcionales) ---

func (s *NarrativeService) generateEvocation(ctx context.Context, userMessage string) string {
	msgLower := strings.ToLower(userMessage)
	if strings.Contains(msgLower, "no hables de") || hasNegationSemantic(msgLower) {
		return ""
	}

	resp, err := s.llmClient.Generate(ctx, fmt.Sprintf(evocationPromptTemplate, userMessage))
	if err == nil {
		clean := strings.TrimSpace(resp)
		if clean != "" {
			return clean
		}
	}

	resp, err = s.llmClient.Generate(ctx, fmt.Sprintf(evocationFallbackPrompt, userMessage))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(resp)
}

func hasNegationSemantic(msgLower string) bool {
	markers := []string{"nunca", "jamás", "ya no", "no me"}
	triggers := []string{"abandon", "funeral", "recuerd", "lluvia"}
	for _, m := range markers {
		if strings.Contains(msgLower, m) {
			for _, t := range triggers {
				if strings.Contains(msgLower, t) {
					return true
				}
			}
		}
	}
	return false
}

func (s *NarrativeService) judgeMemory(ctx context.Context, userMessage, memoryContent string) (bool, string, error) {
	resp, err := s.llmClient.Generate(ctx, fmt.Sprintf(rerankJudgePrompt, userMessage, memoryContent))
	if err != nil {
		return false, "", err
	}
	var out struct {
		Use    bool   `json:"use"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		return false, "", err
	}
	return out.Use, out.Reason, nil
}

func detectActiveCharacters(chars []domain.Character, userMessage string) []domain.Character {
	var out []domain.Character
	msg := strings.ToLower(userMessage)
	for _, c := range chars {
		if strings.Contains(msg, strings.ToLower(c.Name)) {
			out = append(out, c)
		}
	}
	return out
}

func humanizeRelative(t time.Time) string {
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
	return fmt.Sprintf("%d años", months/12)
}
