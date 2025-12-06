package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"clone-llm/internal/domain"
	"clone-llm/internal/repository"
)

type mockMessageRepo struct {
	msgs []domain.Message
	err  error
}

func (m *mockMessageRepo) Create(ctx context.Context, message domain.Message) error {
	return nil
}

func (m *mockMessageRepo) ListBySessionID(ctx context.Context, sessionID string) ([]domain.Message, error) {
	return m.msgs, m.err
}

func TestBasicContextService_GetContext(t *testing.T) {
	t.Run("pocos mensajes", func(t *testing.T) {
		msgs := []domain.Message{
			{Role: "user", Content: "hola", CreatedAt: time.Now().Add(-3 * time.Minute)},
			{Role: "clone", Content: "hola, ¿cómo estás?", CreatedAt: time.Now().Add(-2 * time.Minute)},
			{Role: "user", Content: "bien", CreatedAt: time.Now().Add(-1 * time.Minute)},
		}
		repo := &mockMessageRepo{msgs: msgs}
		svc := NewBasicContextService(repo)

		ctxText, err := svc.GetContext(context.Background(), "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !containsAllInOrder(ctxText, []string{"User: hola", "Clone: hola, ¿cómo estás?", "User: bien"}) {
			t.Fatalf("expected messages in order, got: %s", ctxText)
		}
	})

	t.Run("muchos mensajes recorta a 10", func(t *testing.T) {
		var msgs []domain.Message
		now := time.Now()
		for i := 1; i <= 15; i++ {
			msgs = append(msgs, domain.Message{
				Role:      "user",
				Content:   "msg" + itoa(i),
				CreatedAt: now.Add(time.Duration(i) * time.Minute),
			})
		}
		repo := &mockMessageRepo{msgs: msgs}
		svc := NewBasicContextService(repo)

		ctxText, err := svc.GetContext(context.Background(), "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		lines := strings.Split(ctxText, "\n")
		if len(lines) != 10 {
			t.Fatalf("expected 10 lines, got %d", len(lines))
		}
		if !strings.Contains(lines[0], "msg6") || !strings.Contains(lines[len(lines)-1], "msg15") {
			t.Fatalf("expected context to start at msg6 and end at msg15, got: %s ... %s", lines[0], lines[len(lines)-1])
		}
	})

	t.Run("orden invertido se corrige", func(t *testing.T) {
		now := time.Now()
		msgs := []domain.Message{
			{Role: "clone", Content: "segundo", CreatedAt: now.Add(1 * time.Minute)},
			{Role: "user", Content: "primero", CreatedAt: now},
		}
		repo := &mockMessageRepo{msgs: msgs}
		svc := NewBasicContextService(repo)

		ctxText, err := svc.GetContext(context.Background(), "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "User: primero\nClone: segundo"
		if ctxText != expected {
			t.Fatalf("expected chronological order, got: %s", ctxText)
		}
	})

	t.Run("sin historial", func(t *testing.T) {
		repo := &mockMessageRepo{msgs: []domain.Message{}}
		svc := NewBasicContextService(repo)

		ctxText, err := svc.GetContext(context.Background(), "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ctxText != "" {
			t.Fatalf("expected empty context, got: %q", ctxText)
		}
	})
}

func containsAllInOrder(text string, parts []string) bool {
	idx := 0
	for _, p := range parts {
		pos := indexAfter(text, p, idx)
		if pos == -1 {
			return false
		}
		idx = pos
	}
	return true
}

func indexAfter(text, substr string, start int) int {
	if start < 0 || start >= len(text) {
		start = 0
	}
	pos := strings.Index(text[start:], substr)
	if pos == -1 {
		return -1
	}
	return start + pos + len(substr)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

var _ repository.MessageRepository = (*mockMessageRepo)(nil)
