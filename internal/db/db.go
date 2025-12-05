package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"clone-llm/internal/config"
)

// NewPool construye y devuelve un pool de conexiones configurado.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// Configuración razonable para ambientes iniciales.
	poolCfg.MaxConns = 10
	poolCfg.MinConns = 1
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second
	poolCfg.ConnConfig.ConnectTimeout = 5 * time.Second

	return pgxpool.NewWithConfig(ctx, poolCfg)
}

// Ping verifica conectividad con la base de datos.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	return pool.Ping(ctx)
}
