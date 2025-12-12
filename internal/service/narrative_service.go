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
Estás actuando como el subconsciente de una IA. Tu objetivo NO es responder al usuario, sino generar una "Query de Búsqueda" para encontrar recuerdos relevantes en tu base de datos vectorial.

Mensaje del Usuario: "%s"

Instrucciones:
1. Ignora las palabras de relleno.
2. Identifica la emoción subyacente (miedo, validación, soledad, ira reprimida).
3. Identifica conceptos abstractos asociados (ej: si dice "mi padre me gritó", busca "autoridad", "conflicto", "infancia").
4. Genera una lista de palabras clave y conceptos separados por espacios que representen el "corazón psicológico" del mensaje.

Salida (SOLO TEXTO PLANO):
`

// NarrativeService recupera contexto narrativo relevante para el clon.
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

	embed, err := s.llmClient.CreateEmbedding(ctx, searchQuery)
	if err != nil {
		return "", fmt.Errorf("create embedding: %w", err)
	}

	memories, err := s.memoryRepo.Search(ctx, profileID, pgvector.NewVector(embed), 5)
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}

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
			line := fmt.Sprintf("- [TEMA: %s | Hace %s] %s", strings.ToUpper(label), relative, strings.TrimSpace(m.Content))
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
			line := fmt.Sprintf("- Interlocutor: %s (Relacion: %s, Confianza: %d, Intimidad: %d, Respeto: %d", c.Name, c.Relation, c.Relationship.Trust, c.Relationship.Intimacy, c.Relationship.Respect)
			if strings.TrimSpace(c.BondStatus) != "" {
				line += fmt.Sprintf(", Estado: %s", c.BondStatus)
			}
			line += ")."
			lines = append(lines, line)
		}
		sections = append(sections, "[ESTADO DEL VINCULO]\n"+strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return "", nil
	}

	return strings.Join(sections, "\n\n"), nil
}

// CreateRelation crea un personaje/vinculo asociado al perfil.
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

func humanizeRelative(t time.Time) string {
	if t.IsZero() {
		return "Fecha desconocida"
	}
	d := time.Since(t)
	if d < time.Minute {
		return "Hace instantes"
	}
	if d < time.Hour {
		return fmt.Sprintf("Hace %d minutos", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("Hace %d horas", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	if days < 30 {
		return fmt.Sprintf("Hace %d dias", days)
	}
	months := days / 30
	if months < 12 {
		return fmt.Sprintf("Hace %d meses", months)
	}
	years := months / 12
	return fmt.Sprintf("Hace %d anos", years)
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

	prompt := `Actua como un Analista de Archivos Psicologicos. Analiza el siguiente historial de chat. Genera un JSON valido con:
1) "summary": Un resumen conciso en tercera persona, enfocado en dinamica de relacion y eventos clave.
2) "extracted_facts": Una lista de HECHOS objetivos sobre el usuario o la relacion que se mencionaron explicitamente (nombres, gustos, ubicacion, relaciones).
3) "emotional_shift": Una frase sobre como cambio el tono (ej: "Confianza disminuyo", "Clima emocional se enfrio").`
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
	if len(userMessage) < 10 {
		return userMessage
	}

	prompt := fmt.Sprintf(evocationPromptTemplate, strings.TrimSpace(userMessage))
	resp, err := s.llmClient.Generate(ctx, prompt)
	if err != nil {
		fmt.Printf("warn: generate evocation failed: %v\n", err)
		return userMessage
	}

	cleaned := strings.TrimSpace(resp)
	if cleaned == "" {
		// Si el LLM devolvio vacio, usamos el mensaje original como fallback
		return userMessage
	}
	return cleaned
}
