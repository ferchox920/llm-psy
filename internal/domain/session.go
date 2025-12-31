package domain

import "time"

type Session struct {
	ID           string              `json:"id"`
	UserID       string              `json:"user_id"`
	Token        string              `json:"token"`
	ExpiresAt    time.Time           `json:"expires_at"`
	Relationship RelationshipVectors `json:"relationship"`
	CreatedAt    time.Time           `json:"created_at"`
}
