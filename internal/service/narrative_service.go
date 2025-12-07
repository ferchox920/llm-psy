package service

import (
	"context"
	"fmt"
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
	if len(active) > 0 {
		var lines []string
		for _, c := range active {
			line := fmt.Sprintf("%s (%s", c.Name, c.Relation)
			if strings.TrimSpace(c.BondStatus) != "" {
				line += fmt.Sprintf(", Vinculo: %s", c.BondStatus)
			}
			line += fmt.Sprintf(", Nivel: %d)", c.BondLevel)
			lines = append(lines, line)
		}
		sections = append(sections, "- Personajes Identificados: "+strings.Join(lines, "; "))
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
		var lines []string
		for _, m := range memories {
			relative := humanizeRelative(m.HappenedAt)
			lines = append(lines, fmt.Sprintf("* (%s) %s", relative, strings.TrimSpace(m.Content)))
		}
		sections = append(sections, "- Recuerdos Relevantes:\n  "+strings.Join(lines, "\n  "))
	}

	if len(sections) == 0 {
		return "", nil
	}

	return "[MEMORIA NARRATIVA]\n" + strings.Join(sections, "\n"), nil
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
