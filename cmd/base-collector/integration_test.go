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

func TestBaseCollector_PaymentsTableExists(t *testing.T) {
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
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}

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

	pool, err := db.Open(ctx, connStr)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	// payments table present, empty, with the right shape.
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM payments").Scan(&count); err != nil {
		t.Fatalf("count payments: %v", err)
	}
	if count != 0 {
		t.Fatalf("payments count = %d, want 0", count)
	}

	// composite PK enforced (insert two rows differing only in log_index works).
	insert := `
        INSERT INTO payments (
            chain, tx_hash, log_index, block_number, block_timestamp, source, protocol,
            facilitator, payer, payee,
            asset, token_address, amount_raw, amount_usdc, asset_usd_at_time,
            auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
            gas_used, effective_gas_price, gas_cost_wei
        ) VALUES (
            'base', '0xdead', $1, 1, now(), 'base-collector', 'x402',
            '0xfac', '0xpay', '0xrec',
            'USDC', '0xusdc', 1000000, 1, 1,
            E'\\\\x00', E'\\\\xe3ee160e', '0xusdc', 2, 1,
            50000, 1000000000, 50000000000000
        )`
	if _, err := pool.Exec(ctx, insert, 0); err != nil {
		t.Fatalf("insert row 0: %v", err)
	}
	if _, err := pool.Exec(ctx, insert, 1); err != nil {
		t.Fatalf("insert row 1: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM payments WHERE tx_hash = '0xdead'").Scan(&count); err != nil {
		t.Fatalf("re-count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 multicall rows, got %d", count)
	}

	// collector_cursor present, ready for writes.
	if _, err := pool.Exec(
		ctx,
		"INSERT INTO collector_cursor (collector_name, last_block) VALUES ('base-collector', 100)",
	); err != nil {
		t.Fatalf("insert cursor: %v", err)
	}
	var lastBlock int64
	if err := pool.QueryRow(
		ctx,
		"SELECT last_block FROM collector_cursor WHERE collector_name = 'base-collector'",
	).Scan(&lastBlock); err != nil {
		t.Fatalf("read cursor: %v", err)
	}
	if lastBlock != 100 {
		t.Fatalf("cursor = %d, want 100", lastBlock)
	}
}
