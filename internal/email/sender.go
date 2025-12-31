package email

import (
	"context"
	"errors"
	"time"
)

// Sender define la interfaz para envio de correos de verificacion.
type Sender interface {
	SendVerificationOTP(ctx context.Context, toEmail string, code string, expiresAt time.Time) error
}

type disabledSender struct {
	reason string
}

func NewDisabledSender(reason string) Sender {
	return &disabledSender{reason: reason}
}

func (s *disabledSender) SendVerificationOTP(_ context.Context, _ string, _ string, _ time.Time) error {
	if s.reason == "" {
		return errors.New("email sender disabled")
	}
	return errors.New(s.reason)
}
