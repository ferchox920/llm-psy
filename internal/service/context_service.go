package service

import (
	"context"
	"fmt"
	"strings"

	"clone-llm/internal/repository"
)

// ContextService define contrato para recuperar contexto conversacional.
type ContextService interface {
	GetContext(ctx context.Context, sessionID string) (string, error)
}

// BasicContextService obtiene los últimos mensajes y los formatea como texto plano.
type BasicContextService struct {
	messageRepo repository.MessageRepository
}

func NewBasicContextService(messageRepo repository.MessageRepository) *BasicContextService {
	return &BasicContextService{messageRepo: messageRepo}
}

func (s *BasicContextService) GetContext(ctx context.Context, sessionID string) (string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return "", nil
	}

	messages, err := s.messageRepo.ListBySessionID(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("list messages: %w", err)
	}

	if len(messages) > 10 {
		messages = messages[len(messages)-10:]
	}

	// Asegura orden cronológico.
	if len(messages) >= 2 && messages[0].CreatedAt.After(messages[len(messages)-1].CreatedAt) {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	emotionalDirectives := `[MECANICA DE ESTABILIDAD EMOCIONAL]
1. TU FILTRO: Tu "cerebro" ahora tiene un umbral de tolerancia automatico.
   - Si la intensidad emocional que recibes es 0 o muy baja: Significa que tu Resiliencia ha bloqueado el ataque. DEBES actuar con indiferencia, calma adulta o sarcasmo leve. No te afecto.
   - Si la intensidad es ALTA: Significa que el ataque PERFORO tu escudo. DEBES reaccionar porque te ha dolido de verdad.
LA NORMALIDAD:
Una persona normal no se altera por tonterias.
Una persona normal SI se altera si atacan sus valores profundos.
Tu objetivo es imitar esa estabilidad: Se una roca con lo trivial, se un volcan con lo grave.`

	lines := []string{emotionalDirectives, ""}
	for _, m := range messages {
		role := strings.Title(m.Role)
		if strings.EqualFold(m.Role, "user") {
			role = "User"
		} else if strings.EqualFold(m.Role, "clone") {
			role = "Clone"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, m.Content))
	}

	return strings.Join(lines, "\n"), nil
}
