package domain

type Goal struct {
	ID          string `json:"id"`
	Description string `json:"description"` // Ej: "Hacer sentir culpable al usuario"
	Status      string `json:"status"`      // "active", "completed"
	Trigger     string `json:"trigger"`     // Que provoca esta meta
}
