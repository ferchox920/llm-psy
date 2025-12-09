package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

// NarrativeService recupera contexto narrativo relevante para el clon.
type NarrativeService struct {
	characterRepo repository.CharacterRepository
	memoryRepo    repository.MemoryRepository
	llmClient     llmClientWithEmbedding
}

type llmClientWithEmbedding interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
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

	embed, err := s.llmClient.CreateEmbedding(ctx, userMessage)
	if err != nil {
		return "", fmt.Errorf("create embedding: %w", err)
	}

	memories, err := s.memoryRepo.Search(ctx, profileID, pgvector.NewVector(embed), 5)
	if err != nil {
		return "", fmt.Errorf("search memories: %w", err)
	}

	if len(memories) > 0 {
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
			line := fmt.Sprintf("- [%s: %d] (%s) %s", strings.ToUpper(label), intensity, relative, strings.TrimSpace(m.Content))
			if intensity > 70 {
				traumas = append(traumas, line)
			} else {
				recents = append(recents, line)
			}
		}
		if len(traumas) > 0 {
			sections = append(sections, "=== MEMORIAS DE ALTO IMPACTO EMOCIONAL (INTENSIDAD > 70) ===\n"+strings.Join(traumas, "\n"))
		}
		if len(recents) > 0 {
			sections = append(sections, "=== CONTEXTO RECIENTE (INTENSIDAD BAJA/MEDIA) ===\n"+strings.Join(recents, "\n"))
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
