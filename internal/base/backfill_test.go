//go:build integration

package base_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/db"
)

// fakeFetcher, fakeStream, and fixtureBatch are DB-free helpers shared with the
// fast unit suite — they live in fixtures_test.go (untagged).

func setupStore(t *testing.T) (context.Context, *base.Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pg, err := postgres.Run(
		ctx, "postgres:16-alpine",
		postgres.WithDatabase("fathom_test"),
		postgres.WithUsername("fathom"),
		postgres.WithPassword("fathom"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "../../database/migrations"))
	pool, err := db.Open(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return ctx, base.NewStore(pool)
}

func TestBackfill_Run_WritesPaymentsAndCursor(t *testing.T) {
	ctx, store := setupStore(t)

	f := &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}}
	bf := base.NewBackfiller(f, store)

	require.NoError(t, bf.Run(ctx, 100, 100))

	// One row inserted.
	pool := store.Pool()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&n))
	require.Equal(t, 1, n)

	// tx_hash stored as canonical 32-byte hex (lowercased by common.Hash.Hex()).
	var txHash string
	require.NoError(t, pool.QueryRow(ctx, `SELECT tx_hash FROM payments LIMIT 1`).Scan(&txHash))
	require.Equal(t, common.HexToHash("0xdead").Hex(), txHash)

	// Cursor advanced to max block (100).
	cur, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cur)
}

func TestBackfill_Run_EmptyBatchDoesNotResetCursor(t *testing.T) {
	ctx, store := setupStore(t)

	// Prime the cursor at 999 via a non-empty batch first.
	first := fixtureBatch()
	first.Data.Blocks[0].Number = 999
	first.Data.Logs[0].BlockNumber = 999
	first.Data.Logs[1].BlockNumber = 999
	first.Data.Transactions[0].BlockNumber = 999

	empty := base.HyperSyncBatch{NextBlock: 1000} // no blocks, MaxBlock == 0

	f := &fakeFetcher{batches: []base.HyperSyncBatch{first, empty}}
	bf := base.NewBackfiller(f, store)
	require.NoError(t, bf.Run(ctx, 999, 999))

	cur, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(999), cur, "empty batch must not reset cursor to 0")
}

func TestBackfill_Run_Idempotent(t *testing.T) {
	ctx, store := setupStore(t)

	f := &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}}
	require.NoError(t, base.NewBackfiller(f, store).Run(ctx, 100, 100))

	// Re-run with the same batch — no duplicate rows.
	f2 := &fakeFetcher{batches: []base.HyperSyncBatch{fixtureBatch()}}
	require.NoError(t, base.NewBackfiller(f2, store).Run(ctx, 100, 100))

	pool := store.Pool()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&n))
	require.Equal(t, 1, n)
}

func TestBackfill_Run_AllowCandidateLossAdvances(t *testing.T) {
	ctx, store := setupStore(t)

	// Same poisoned shape that halts the run by default (see
	// TestBackfill_Run_HaltsWhenAllCandidatesDrop): with the explicit escape
	// hatch the batch commits empty and the cursor steps past it.
	f := &fakeFetcher{batches: []base.HyperSyncBatch{candidateNoCompanionBatch()}}
	bf := base.NewBackfiller(f, store, base.AllowCandidateLoss())

	require.NoError(t, bf.Run(ctx, 100, 100))

	cur, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cur, "cursor must advance past the lossy batch when explicitly allowed")
}

func TestStore_AssertSchema(t *testing.T) {
	ctx, store := setupStore(t)

	// Fully migrated schema: amount_usdc is GENERATED — the check passes.
	require.NoError(t, store.AssertSchema(ctx))

	// Simulate a pre-00008 schema (plain column): the check must fail with a
	// migration pointer instead of letting InsertBatch error per batch.
	_, err := store.Pool().Exec(ctx, `ALTER TABLE payments ALTER COLUMN amount_usdc DROP EXPRESSION`)
	require.NoError(t, err)
	require.ErrorContains(t, store.AssertSchema(ctx), "GENERATED")
}
