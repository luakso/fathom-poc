//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql, used by goose
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/db"
)

func TestBaseCollector_OpensPoolAgainstRealPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, err := postgres.Run(
		ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("fathom_test"),
		postgres.WithUsername("fathom"),
		postgres.WithPassword("fathom"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = pg.Terminate(ctx)
	})

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}

	// Apply goose migrations using database/sql + pgx stdlib driver.
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose SetDialect: %v", err)
	}
	if err := goose.Up(sqlDB, "../../database/migrations"); err != nil {
		t.Fatalf("goose Up: %v", err)
	}

	// Now exercise the unit under test.
	pool, err := db.Open(ctx, connStr)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}

	var initialized string
	row := pool.QueryRow(ctx, "SELECT value FROM _fathom_meta WHERE key = 'schema_initialized'")
	if err := row.Scan(&initialized); err != nil {
		t.Fatalf("read _fathom_meta: %v", err)
	}
	if initialized != "true" {
		t.Fatalf("_fathom_meta schema_initialized = %q, want true", initialized)
	}
}
