package domain

import (
	"time"

	"github.com/google/uuid"
	pgvector "github.com/pgvector/pgvector-go"
)

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CloneProfile struct {
	ID          string      `json:"id"`
	UserID      string      `json:"user_id"`
	Name        string      `json:"name"`
	Bio         string      `json:"bio,omitempty"`
	Big5        Big5Profile `json:"big5"`
	CurrentGoal *Goal       `json:"current_goal,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

type Big5Profile struct {
	Openness          int `json:"openness"`          // Creatividad vs. Pragmatismo
	Conscientiousness int `json:"conscientiousness"` // Orden vs. Caos
	Extraversion      int `json:"extraversion"`      // Energía social
	Agreeableness     int `json:"agreeableness"`     // Amabilidad (Ya usado, ahora formalizado)
	Neuroticism       int `json:"neuroticism"`       // Estabilidad (Ya usado, ahora formalizado)
}

// GetResilience calcula un factor de 0.0 (Cristal) a 1.0 (Tanque)
// Basado en: Bajo Neuroticismo (+), Alta Consciencia (+), Alta Extraversión (+)
func (p *CloneProfile) GetResilience() float64 {
	// Neuroticismo es el factor inverso principal (0-100)
	stability := float64(100 - p.Big5.Neuroticism)

	// Consciencia ayuda a racionalizar (0-100)
	coping := float64(p.Big5.Conscientiousness)

	// Extraversión da red de apoyo/energía (0-100)
	energy := float64(p.Big5.Extraversion)

	// Ponderación: 60% Estabilidad, 25% Coping, 15% Energía
	score := (stability * 0.6) + (coping * 0.25) + (energy * 0.15)

	// Normalizar a factor 0.0 - 1.0 (dividiendo por 100)
	return score / 100.0
}

type Goal struct {
	ID          string `json:"id"`
	Description string `json:"description"` // Ej: "Hacer sentir culpable al usuario"
	Status      string `json:"status"`      // "active", "completed"
	Trigger     string `json:"trigger"`     // Qué provocó esta meta
}

type Session struct {
	ID           string              `json:"id"`
	UserID       string              `json:"user_id"`
	Token        string              `json:"token"`
	ExpiresAt    time.Time           `json:"expires_at"`
	Relationship RelationshipVectors `json:"relationship"`
	CreatedAt    time.Time           `json:"created_at"`
}

type Message struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id,omitempty"`
	Content   string    `json:"content"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

const (
	TraitCategoryBigFive    = "BIG_FIVE"
	TraitCategoryValues     = "VALUES"
	TraitCategoryAttachment = "ATTACHMENT"
)

type Trait struct {
	ID         string    `json:"id"`
	ProfileID  string    `json:"profile_id"`
	Category   string    `json:"category"`
	Trait      string    `json:"trait"`
	Value      int       `json:"value"`
	Confidence *float64  `json:"confidence,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

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

// InteractionDebug expone datos intermedios para pruebas/telemetría.
type InteractionDebug struct {
	InputIntensity      float64 `json:"input_intensity"`
	CloneResilience     float64 `json:"clone_resilience"`
	ActivationThreshold float64 `json:"activation_threshold"`
	EffectiveIntensity  float64 `json:"effective_intensity"`
	IsTriggered         bool    `json:"is_triggered"`
}

// MemoryConsolidation combina narrativa y hechos concretos extraídos de una conversación.
type MemoryConsolidation struct {
	Summary  string   `json:"summary"`
	NewFacts []string `json:"new_facts"`
}

// NarrativeOutput es la estructura esperada del LLM al analizar una sesión.
type NarrativeOutput struct {
	Summary        string   `json:"summary"`         // Resumen narrativo de lo sucedido (3ra persona)
	ExtractedFacts []string `json:"extracted_facts"` // Lista de datos duros nuevos o hechos relevantes
	EmotionalShift string   `json:"emotional_shift"` // Cambio en la dinámica
}
