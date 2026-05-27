//go:build integration

package base_test

import (
	"context"
	"database/sql"
	"math/big"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/db"
	"github.com/lukostrobl/fathom/internal/x402"
)

func setup(t *testing.T) (context.Context, *base.Store) {
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

func samplePayment(logIndex uint32) x402.Payment {
	return x402.Payment{
		Chain:             x402.ChainBase,
		TxHash:            "0xdeadbeef",
		LogIndex:          logIndex,
		BlockNumber:       100,
		BlockTimestamp:    time.Unix(1_700_000_000, 0).UTC(),
		Source:            "base-collector",
		Protocol:          "x402",
		Facilitator:       "0xfac",
		Payer:             "0xpay",
		Payee:             "0xrec",
		Asset:             "USDC",
		TokenAddress:      strings.ToLower(x402.USDCProxyBase.Hex()),
		AmountRaw:         big.NewInt(1_000_000),
		AmountUSDC:        decimal.NewFromInt(1),
		AssetUSDAtTime:    decimal.NewFromInt(1),
		AuthNonce:         []byte{0x01, 0x02},
		MethodSelector:    []byte{0xe3, 0xee, 0x16, 0x0e},
		CalledContract:    strings.ToLower(x402.USDCProxyBase.Hex()),
		TxType:            2,
		TxNonce:           42,
		GasUsed:           50_000,
		EffectiveGasPrice: big.NewInt(1_000_000_000),
		GasCostWei:        big.NewInt(50_000_000_000_000),
		BaseFeePerGas:     big.NewInt(500_000_000),
	}
}

func TestStore_InsertBatch_WritesAndAdvancesCursor(t *testing.T) {
	ctx, store := setup(t)

	batch := []x402.Payment{samplePayment(1), samplePayment(2)}
	require.NoError(t, store.InsertBatch(ctx, batch, 100))

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor)
}

func TestStore_InsertBatch_Idempotent(t *testing.T) {
	ctx, store := setup(t)

	batch := []x402.Payment{samplePayment(1)}
	require.NoError(t, store.InsertBatch(ctx, batch, 100))
	require.NoError(t, store.InsertBatch(ctx, batch, 100), "re-insert must not error")

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor)
}

func TestStore_InsertBatch_CursorMonotonic(t *testing.T) {
	ctx, store := setup(t)

	require.NoError(t, store.InsertBatch(ctx, nil, 200))
	require.NoError(t, store.InsertBatch(ctx, nil, 150), "cursor must not regress")

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(200), cursor, "cursor stayed at the max, not the later-but-smaller write")
}

func TestStore_InsertBatch_RollsBackOnError(t *testing.T) {
	ctx, store := setup(t)

	// Same-batch duplicates are dropped by ON CONFLICT, so this succeeds.
	dup := []x402.Payment{samplePayment(1), samplePayment(1)}
	require.NoError(t, store.InsertBatch(ctx, dup, 999))

	// Now the rollback test: a row with NumericOverflow on amount_raw.
	oversize := samplePayment(2)
	oversize.AmountRaw = new(big.Int).Exp(big.NewInt(10), big.NewInt(80), nil) // 10^80, exceeds NUMERIC(78,0)
	err := store.InsertBatch(ctx, []x402.Payment{oversize}, 5000)
	require.Error(t, err)

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(999), cursor, "cursor must NOT advance to 5000; batch rolled back")
}

func TestStore_GetCursor_EmptyReturnsZero(t *testing.T) {
	ctx, store := setup(t)

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(0), cursor, "empty cursor table reports 0, treated as 'never synced'")
}
