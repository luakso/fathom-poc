//go:build integration

package base_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/db"
	"github.com/lukostrobl/fathom/internal/x402"
)

// fakeFetcher returns a fixed sequence of batches.
type fakeFetcher struct{ batches []base.HyperSyncBatch }

func (f *fakeFetcher) Stream(_ base.HyperSyncQuery) (base.Stream, error) {
	return &fakeStream{batches: f.batches}, nil
}

type fakeStream struct {
	batches []base.HyperSyncBatch
	idx     int
}

func (s *fakeStream) Next() (base.HyperSyncBatch, bool, error) {
	if s.idx >= len(s.batches) {
		return base.HyperSyncBatch{}, false, nil
	}
	b := s.batches[s.idx]
	s.idx++
	return b, true, nil
}
func (s *fakeStream) Close() error { return nil }

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

// fixtureBatch builds a HyperSyncBatch representing one classic-sig
// transferWithAuthorization in block 100. Returns the batch and the expected
// tx hash for downstream assertions.
func fixtureBatch() base.HyperSyncBatch {
	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	return base.HyperSyncBatch{
		Data: base.HyperSyncBatchData{
			Logs: []base.HyperSyncLog{
				{
					Address: strings.ToLower(x402.USDCProxyBase.Hex()),
					Topics: []string{
						x402.TransferTopic.Hex(),
						payer, payee,
					},
					Data:        "0x00000000000000000000000000000000000000000000000000000000000f4240", // 1 USDC
					BlockNumber: 100,
					TxHash:      "0xdead",
					TxIndex:     0,
					LogIndex:    0,
				},
				{
					Address: strings.ToLower(x402.USDCProxyBase.Hex()),
					Topics: []string{
						x402.AuthorizationUsedTopic.Hex(),
						payer,
					},
					Data:        "0x1111111111111111111111111111111111111111111111111111111111111111",
					BlockNumber: 100,
					TxHash:      "0xdead",
					TxIndex:     0,
					LogIndex:    1,
				},
			},
			Transactions: []base.HyperSyncTransaction{
				{
					Hash:              "0xdead",
					BlockNumber:       100,
					From:              "0xfac1000000000000000000000000000000000001",
					To:                strings.ToLower(x402.USDCProxyBase.Hex()),
					Input:             "0xe3ee160edeadbeef",
					Type:              2,
					Nonce:             7,
					GasUsed:           50_000,
					EffectiveGasPrice: "0x3b9aca00",
					BaseFeePerGas:     "0x1dcd6500",
				},
			},
			Blocks: []base.HyperSyncBlock{
				{Number: 100, Timestamp: 1_700_000_000, Hash: "0xb100"},
			},
		},
		NextBlock: 101,
	}
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
