package domain

import "time"

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	DisplayName     string     `json:"display_name,omitempty"`
	AuthProvider    string     `json:"auth_provider,omitempty"`
	AuthSubject     string     `json:"-"`
	PasswordHash    string     `json:"-"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	OtpCodeHash     string     `json:"-"`
	OtpExpiresAt    *time.Time `json:"otp_expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}
