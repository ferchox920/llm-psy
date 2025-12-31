package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/domain"
)

// UserRepository define el contrato de persistencia para usuarios.
type UserRepository interface {
	Create(ctx context.Context, user domain.User) error
	GetByID(ctx context.Context, id string) (domain.User, error)
	GetByEmail(ctx context.Context, email string) (domain.User, error)
	GetByAuth(ctx context.Context, provider, subject string) (domain.User, error)
	UpdateOTP(ctx context.Context, id, otpHash string, otpExpiresAt time.Time) error
	VerifyEmail(ctx context.Context, id string, verifiedAt time.Time) error
	LinkOAuth(ctx context.Context, id, provider, subject string) error
}

// PgUserRepository implementa UserRepository usando pgxpool.
type PgUserRepository struct {
	pool *pgxpool.Pool
}

func NewPgUserRepository(pool *pgxpool.Pool) *PgUserRepository {
	return &PgUserRepository{pool: pool}
}

func (r *PgUserRepository) Create(ctx context.Context, user domain.User) error {
	const query = `
		INSERT INTO users (
			id,
			email,
			display_name,
			auth_provider,
			auth_subject,
			password_hash,
			email_verified_at,
			otp_code_hash,
			otp_expires_at,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := r.pool.Exec(ctx, query,
		user.ID,
		user.Email,
		user.DisplayName,
		user.AuthProvider,
		user.AuthSubject,
		user.PasswordHash,
		user.EmailVerifiedAt,
		user.OtpCodeHash,
		user.OtpExpiresAt,
		user.CreatedAt,
	)
	return err
}

func (r *PgUserRepository) GetByID(ctx context.Context, id string) (domain.User, error) {
	const query = `
		SELECT
			id,
			email,
			display_name,
			auth_provider,
			auth_subject,
			password_hash,
			email_verified_at,
			otp_code_hash,
			otp_expires_at,
			created_at
		FROM users
		WHERE id = $1
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&u.ID,
		&u.Email,
		&u.DisplayName,
		&u.AuthProvider,
		&u.AuthSubject,
		&u.PasswordHash,
		&u.EmailVerifiedAt,
		&u.OtpCodeHash,
		&u.OtpExpiresAt,
		&u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}
	return u, err
}

func (r *PgUserRepository) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	const query = `
		SELECT
			id,
			email,
			display_name,
			auth_provider,
			auth_subject,
			password_hash,
			email_verified_at,
			otp_code_hash,
			otp_expires_at,
			created_at
		FROM users
		WHERE email = $1
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&u.ID,
		&u.Email,
		&u.DisplayName,
		&u.AuthProvider,
		&u.AuthSubject,
		&u.PasswordHash,
		&u.EmailVerifiedAt,
		&u.OtpCodeHash,
		&u.OtpExpiresAt,
		&u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}
	return u, err
}

func (r *PgUserRepository) GetByAuth(ctx context.Context, provider, subject string) (domain.User, error) {
	const query = `
		SELECT
			id,
			email,
			display_name,
			auth_provider,
			auth_subject,
			password_hash,
			email_verified_at,
			otp_code_hash,
			otp_expires_at,
			created_at
		FROM users
		WHERE auth_provider = $1 AND auth_subject = $2
	`
	var u domain.User
	err := r.pool.QueryRow(ctx, query, provider, subject).Scan(
		&u.ID,
		&u.Email,
		&u.DisplayName,
		&u.AuthProvider,
		&u.AuthSubject,
		&u.PasswordHash,
		&u.EmailVerifiedAt,
		&u.OtpCodeHash,
		&u.OtpExpiresAt,
		&u.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}
	return u, err
}

func (r *PgUserRepository) UpdateOTP(ctx context.Context, id, otpHash string, otpExpiresAt time.Time) error {
	const query = `
		UPDATE users
		SET otp_code_hash = $1, otp_expires_at = $2
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, otpHash, otpExpiresAt, id)
	return err
}

func (r *PgUserRepository) VerifyEmail(ctx context.Context, id string, verifiedAt time.Time) error {
	const query = `
		UPDATE users
		SET email_verified_at = $1,
			otp_code_hash = NULL,
			otp_expires_at = NULL
		WHERE id = $2
	`
	_, err := r.pool.Exec(ctx, query, verifiedAt, id)
	return err
}

func (r *PgUserRepository) LinkOAuth(ctx context.Context, id, provider, subject string) error {
	const query = `
		UPDATE users
		SET auth_provider = $1, auth_subject = $2
		WHERE id = $3
	`
	_, err := r.pool.Exec(ctx, query, provider, subject, id)
	return err
}
