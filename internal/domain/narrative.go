package domain

import (
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

// Narrative structures
// Usamos uuid.UUID para IDs y pgvector.Vector para embeddings vectoriales.
type Character struct {
	ID             uuid.UUID           `json:"id"`
	CloneProfileID uuid.UUID           `json:"clone_profile_id"`
	Name           string              `json:"name"`
	Relation       string              `json:"relation"`
	Archetype      string              `json:"archetype"`
	BondStatus     string              `json:"bond_status"`
	Relationship   RelationshipVectors `json:"relationship"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

type NarrativeMemory struct {
	ID                 uuid.UUID       `json:"id"`
	CloneProfileID     uuid.UUID       `json:"clone_profile_id"`
	RelatedCharacterID *uuid.UUID      `json:"related_character_id,omitempty"`
	Content            string          `json:"content"`
	Embedding          pgvector.Vector `json:"embedding"`
	Importance         int             `json:"importance"`
	EmotionalWeight    int             `json:"emotional_weight"` // 1-10 escala de carga emocional
	EmotionalIntensity int             `json:"emotional_intensity"`
	EmotionCategory    string          `json:"emotion_category"`
	SentimentLabel     string          `json:"sentiment_label"` // Ira, Alegria, Miedo, etc.
	HappenedAt         time.Time       `json:"happened_at"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type RelationshipVectors struct {
	Trust    int `json:"trust"`    // Confianza
	Intimacy int `json:"intimacy"` // Carino/Afecto
	Respect  int `json:"respect"`  // Respeto profesional/intelectual
}

// LLMResponse representa la salida estructurada esperada del LLM generador.
type LLMResponse struct {
	InnerMonologue string  `json:"inner_monologue"`
	PublicResponse string  `json:"public_response"`
	TrustDelta     float64 `json:"trust_delta"`
	IntimacyDelta  float64 `json:"intimacy_delta"`
	RespectDelta   float64 `json:"respect_delta"`
	NewState       string  `json:"new_state"`
}

// InteractionDebug expone datos intermedios para pruebas/telemetria.
type InteractionDebug struct {
	InputIntensity      float64 `json:"input_intensity"`
	CloneResilience     float64 `json:"clone_resilience"`
	ActivationThreshold float64 `json:"activation_threshold"`
	EffectiveIntensity  float64 `json:"effective_intensity"`
	IsTriggered         bool    `json:"is_triggered"`
}

// MemoryConsolidation combina narrativa y hechos concretos extraidos de una conversacion.
type MemoryConsolidation struct {
	Summary  string   `json:"summary"`
	NewFacts []string `json:"new_facts"`
}

// NarrativeOutput es la estructura esperada del LLM al analizar una sesion.
type NarrativeOutput struct {
	Summary        string   `json:"summary"`         // Resumen narrativo de lo sucedido (3ra persona)
	ExtractedFacts []string `json:"extracted_facts"` // Lista de datos duros nuevos o hechos relevantes
	EmotionalShift string   `json:"emotional_shift"` // Cambio en la dinamica
}
