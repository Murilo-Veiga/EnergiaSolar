package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"energiasolar-api/migrations"
)

// Connect abre o pool de conexões com o Postgres.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("conectando ao postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping no postgres: %w", err)
	}
	return pool, nil
}

// Migrate aplica todas as migrations pendentes (embutidas no binário via
// embed.FS, ver migrations/embed.go) — o serviço sobe já com o schema em
// dia, sem precisar de um container/CLI de migrate separado.
func Migrate(databaseURL string) error {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("lendo migrations embutidas: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		return fmt.Errorf("preparando migrate: %w", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("aplicando migrations: %w", err)
	}
	return nil
}
