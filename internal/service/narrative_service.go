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
			weight := m.EmotionalWeight
			if weight <= 0 {
				weight = m.Importance
			}
			if weight < 1 {
				weight = 1
			}
			if weight > 10 {
				weight = 10
			}
			label := strings.TrimSpace(m.SentimentLabel)
			if label == "" {
				label = "Neutral"
			}
			line := fmt.Sprintf("- (%s | peso emocional %d/10 | %s): %s", relative, weight, label, strings.TrimSpace(m.Content))
			if weight >= 8 {
				traumas = append(traumas, line)
			} else {
				recents = append(recents, line)
			}
		}
		if len(traumas) > 0 {
			sections = append(sections, "=== TRAUMAS Y HECHOS CENTRALES (Inolvidables) ===\n"+strings.Join(traumas, "\n"))
		}
		if len(recents) > 0 {
			sections = append(sections, "=== EVENTOS RECIENTES (Contexto temporal) ===\n"+strings.Join(recents, "\n"))
		}
	}

	if len(active) > 0 {
		var lines []string
		for _, c := range active {
			line := fmt.Sprintf("- Interlocutor: %s (Relacion: %s, Nivel: %d/100", c.Name, c.Relation, c.BondLevel)
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
func (s *NarrativeService) CreateRelation(ctx context.Context, profileID uuid.UUID, name, relation, bondStatus string, level int) error {
	now := time.Now().UTC()
	char := domain.Character{
		ID:             uuid.New(),
		CloneProfileID: profileID,
		Name:           strings.TrimSpace(name),
		Relation:       strings.TrimSpace(relation),
		BondStatus:     strings.TrimSpace(bondStatus),
		BondLevel:      level,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return s.characterRepo.Create(ctx, char)
}

// InjectMemory genera el embedding y guarda una memoria narrativa.
// emotionalWeight escala 1-10 para intensidad afectiva, sentimentLabel describe el tono (Ira, Alegria, Miedo, etc).
func (s *NarrativeService) InjectMemory(ctx context.Context, profileID uuid.UUID, content string, importance, emotionalWeight int, sentimentLabel string) error {
	embed, err := s.llmClient.CreateEmbedding(ctx, content)
	if err != nil {
		return fmt.Errorf("create embedding: %w", err)
	}

	now := time.Now().UTC()
	mem := domain.NarrativeMemory{
		ID:                 uuid.New(),
		CloneProfileID:     profileID,
		Content:            strings.TrimSpace(content),
		Embedding:          pgvector.NewVector(embed),
		Importance:         importance,
		EmotionalWeight:    emotionalWeight,
		SentimentLabel:     strings.TrimSpace(sentimentLabel),
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
