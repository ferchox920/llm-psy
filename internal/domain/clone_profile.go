package domain

import "time"

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
	Extraversion      int `json:"extraversion"`      // Energia social
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

	// Extraversión da red de apoyo/energia (0-100)
	energy := float64(p.Big5.Extraversion)

	// Ponderacion: 60% Estabilidad, 25% Coping, 15% Energia
	score := (stability * 0.6) + (coping * 0.25) + (energy * 0.15)

	// Normalizar a factor 0.0 - 1.0 (dividiendo por 100)
	return score / 100.0
}
