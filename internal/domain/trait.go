package domain

import "time"

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
