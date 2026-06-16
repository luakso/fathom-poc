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
	require.NoError(t, store.InsertBatch(ctx, batch, nil, 100))

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor)
}

func TestStore_InsertBatch_Idempotent(t *testing.T) {
	ctx, store := setup(t)

	batch := []x402.Payment{samplePayment(1)}
	require.NoError(t, store.InsertBatch(ctx, batch, nil, 100))
	require.NoError(t, store.InsertBatch(ctx, batch, nil, 100), "re-insert must not error")

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor)
}

func TestStore_InsertBatch_CursorMonotonic(t *testing.T) {
	ctx, store := setup(t)

	require.NoError(t, store.InsertBatch(ctx, nil, nil, 200))
	require.NoError(t, store.InsertBatch(ctx, nil, nil, 150), "cursor must not regress")

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(200), cursor, "cursor stayed at the max, not the later-but-smaller write")
}

func TestStore_InsertBatch_RollsBackOnError(t *testing.T) {
	ctx, store := setup(t)

	// Same-batch duplicates are dropped by ON CONFLICT, so this succeeds.
	dup := []x402.Payment{samplePayment(1), samplePayment(1)}
	require.NoError(t, store.InsertBatch(ctx, dup, nil, 999))

	// Now the rollback test: a row with NumericOverflow on amount_raw.
	oversize := samplePayment(2)
	oversize.AmountRaw = new(big.Int).Exp(big.NewInt(10), big.NewInt(80), nil) // 10^80, exceeds NUMERIC(78,0)
	err := store.InsertBatch(ctx, []x402.Payment{oversize}, nil, 5000)
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

// storedPayment mirrors the columns read back from the payments table for
// round-trip assertions. NUMERIC columns come back as shopspring decimals so
// equality is compared numerically (scale-insensitive), proving no precision
// is lost across the COPY → cast path.
type storedPayment struct {
	Chain                string
	TxHash               string
	LogIndex             uint32
	BlockNumber          uint64
	BlockTimestamp       time.Time
	ObservedAt           time.Time
	Source               string
	Protocol             string
	Facilitator          string
	Payer                string
	Payee                string
	PayeeServiceID       *int64
	Asset                string
	TokenAddress         string
	AmountRaw            decimal.Decimal
	AmountUSDC           decimal.Decimal
	AssetUSDAtTime       decimal.Decimal
	AuthNonce            []byte
	MethodSelector       []byte
	CalledContract       string
	TxType               uint8
	TxNonce              uint64
	GasUsed              uint64
	EffectiveGasPrice    decimal.Decimal
	GasCostWei           decimal.Decimal
	BaseFeePerGas        *decimal.Decimal
	MaxFeePerGas         *decimal.Decimal
	MaxPriorityFeePerGas *decimal.Decimal
}

// readPayment fetches one payments row by its PK. NUMERIC columns are cast to
// text in SQL and parsed into decimals here, sidestepping any codec ambiguity.
func readPayment(ctx context.Context, t *testing.T, store *base.Store, txHash string, logIndex uint32) storedPayment {
	t.Helper()

	var (
		out                                                   storedPayment
		logIdx                                                int32
		blockNumber, txNonce, gasUsed                         int64
		txType                                                int16
		amountRaw, amountUSDC, assetUSD, effGasPrice, gasCost string
		baseFee, maxFee, maxPriorityFee                       *string
	)
	err := store.Pool().QueryRow(ctx, `
        SELECT
            chain, tx_hash, log_index, block_number, block_timestamp, observed_at,
            source, protocol, facilitator, payer, payee, payee_service_id,
            asset, token_address,
            amount_raw::text, amount_usdc::text, asset_usd_at_time::text,
            auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
            gas_used, effective_gas_price::text, gas_cost_wei::text, base_fee_per_gas::text,
            max_fee_per_gas::text, max_priority_fee_per_gas::text
        FROM payments
        WHERE chain = $1 AND tx_hash = $2 AND log_index = $3
    `, string(x402.ChainBase), txHash, int32(logIndex)).Scan(
		&out.Chain, &out.TxHash, &logIdx, &blockNumber, &out.BlockTimestamp, &out.ObservedAt,
		&out.Source, &out.Protocol, &out.Facilitator, &out.Payer, &out.Payee, &out.PayeeServiceID,
		&out.Asset, &out.TokenAddress,
		&amountRaw, &amountUSDC, &assetUSD,
		&out.AuthNonce, &out.MethodSelector, &out.CalledContract, &txType, &txNonce,
		&gasUsed, &effGasPrice, &gasCost, &baseFee,
		&maxFee, &maxPriorityFee,
	)
	require.NoError(t, err)

	out.LogIndex = uint32(logIdx)
	out.BlockNumber = uint64(blockNumber)
	out.TxType = uint8(txType)
	out.TxNonce = uint64(txNonce)
	out.GasUsed = uint64(gasUsed)
	out.AmountRaw = decimal.RequireFromString(amountRaw)
	out.AmountUSDC = decimal.RequireFromString(amountUSDC)
	out.AssetUSDAtTime = decimal.RequireFromString(assetUSD)
	out.EffectiveGasPrice = decimal.RequireFromString(effGasPrice)
	out.GasCostWei = decimal.RequireFromString(gasCost)
	if baseFee != nil {
		d := decimal.RequireFromString(*baseFee)
		out.BaseFeePerGas = &d
	}
	if maxFee != nil {
		d := decimal.RequireFromString(*maxFee)
		out.MaxFeePerGas = &d
	}
	if maxPriorityFee != nil {
		d := decimal.RequireFromString(*maxPriorityFee)
		out.MaxPriorityFeePerGas = &d
	}
	return out
}

func countPayments(ctx context.Context, t *testing.T, store *base.Store) int64 {
	t.Helper()
	var n int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM payments`).Scan(&n))
	return n
}

// TestStore_InsertBatch_ColumnsRoundTripExactly proves every column survives the
// insert path with full precision: a 30-digit amount_raw, sub-cent USDC, an
// 8-decimal price, bytea fields, a non-nil payee_service_id, and observed_at
// defaulting to now(). A second row exercises the nullable base_fee path.
func TestStore_InsertBatch_ColumnsRoundTripExactly(t *testing.T) {
	ctx, store := setup(t)

	full := samplePayment(1)
	full.AmountRaw, _ = new(big.Int).SetString("123456789012345678901234567890", 10)
	full.AmountUSDC = decimal.RequireFromString("123456789012345678901234.567890")
	full.AssetUSDAtTime = decimal.RequireFromString("0.99980001")
	full.EffectiveGasPrice = big.NewInt(1_234_567_890)
	full.GasCostWei, _ = new(big.Int).SetString("987654321098765432109876", 10)
	full.BaseFeePerGas = big.NewInt(424_242)
	full.MaxFeePerGas = big.NewInt(2_000_000_000)
	full.MaxPriorityFeePerGas = big.NewInt(1_500_000)
	svc := int64(7)
	full.PayeeServiceID = &svc

	legacy := samplePayment(2) // EIP-2930/legacy-style: no 1559 fee market
	legacy.BaseFeePerGas = nil
	legacy.MaxFeePerGas = nil
	legacy.MaxPriorityFeePerGas = nil
	legacy.PayeeServiceID = nil

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{full, legacy}, nil, 100))

	got := readPayment(ctx, t, store, full.TxHash, full.LogIndex)
	require.Equal(t, full.Chain, got.Chain)
	require.Equal(t, full.TxHash, got.TxHash)
	require.Equal(t, full.LogIndex, got.LogIndex)
	require.Equal(t, full.BlockNumber, got.BlockNumber)
	require.True(t, full.BlockTimestamp.Equal(got.BlockTimestamp), "block_timestamp round-trips")
	require.False(t, got.ObservedAt.IsZero(), "observed_at defaulted to now()")
	require.Equal(t, full.Source, got.Source)
	require.Equal(t, full.Protocol, got.Protocol)
	require.Equal(t, full.Facilitator, got.Facilitator)
	require.Equal(t, full.Payer, got.Payer)
	require.Equal(t, full.Payee, got.Payee)
	require.NotNil(t, got.PayeeServiceID)
	require.Equal(t, *full.PayeeServiceID, *got.PayeeServiceID)
	require.Equal(t, full.Asset, got.Asset)
	require.Equal(t, full.TokenAddress, got.TokenAddress)
	require.True(t, decimal.NewFromBigInt(full.AmountRaw, 0).Equal(got.AmountRaw), "amount_raw exact: %s vs %s", full.AmountRaw, got.AmountRaw)
	require.True(t, full.AmountUSDC.Equal(got.AmountUSDC), "amount_usdc exact: %s vs %s", full.AmountUSDC, got.AmountUSDC)
	require.True(t, full.AssetUSDAtTime.Equal(got.AssetUSDAtTime), "asset_usd_at_time exact")
	require.Equal(t, full.AuthNonce, got.AuthNonce)
	require.Equal(t, full.MethodSelector, got.MethodSelector)
	require.Equal(t, full.CalledContract, got.CalledContract)
	require.Equal(t, full.TxType, got.TxType)
	require.Equal(t, full.TxNonce, got.TxNonce)
	require.Equal(t, full.GasUsed, got.GasUsed)
	require.True(t, decimal.NewFromBigInt(full.EffectiveGasPrice, 0).Equal(got.EffectiveGasPrice), "effective_gas_price exact")
	require.True(t, decimal.NewFromBigInt(full.GasCostWei, 0).Equal(got.GasCostWei), "gas_cost_wei exact")
	require.NotNil(t, got.BaseFeePerGas)
	require.True(t, decimal.NewFromBigInt(full.BaseFeePerGas, 0).Equal(*got.BaseFeePerGas), "base_fee_per_gas exact")
	require.NotNil(t, got.MaxFeePerGas)
	require.True(t, decimal.NewFromBigInt(full.MaxFeePerGas, 0).Equal(*got.MaxFeePerGas), "max_fee_per_gas exact")
	require.NotNil(t, got.MaxPriorityFeePerGas)
	require.True(t, decimal.NewFromBigInt(full.MaxPriorityFeePerGas, 0).Equal(*got.MaxPriorityFeePerGas), "max_priority_fee_per_gas exact")

	legacyGot := readPayment(ctx, t, store, legacy.TxHash, legacy.LogIndex)
	require.Nil(t, legacyGot.BaseFeePerGas, "nullable base_fee_per_gas stays NULL")
	require.Nil(t, legacyGot.MaxFeePerGas, "nullable max_fee_per_gas stays NULL")
	require.Nil(t, legacyGot.MaxPriorityFeePerGas, "nullable max_priority_fee_per_gas stays NULL")
	require.Nil(t, legacyGot.PayeeServiceID, "nullable payee_service_id stays NULL")
}

// TestStore_AmountUSDC_IsGenerated proves amount_usdc is derived by the database
// from amount_raw (migration 00008), not by the writer: a deliberately wrong
// Go-side AmountUSDC is discarded and the stored value is amount_raw / 10^6.
func TestStore_AmountUSDC_IsGenerated(t *testing.T) {
	ctx, store := setup(t)

	p := samplePayment(1)
	p.AmountRaw = big.NewInt(2_500_000)    // 2.5 USDC
	p.AmountUSDC = decimal.NewFromInt(999) // wrong on purpose — must be ignored

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{p}, nil, 100))

	got := readPayment(ctx, t, store, p.TxHash, p.LogIndex)
	require.Equal(t, "2.500000", got.AmountUSDC.StringFixed(6),
		"amount_usdc is GENERATED from amount_raw, not the writer-supplied value")
}

// TestStore_InsertBatch_MixedNewAndExisting verifies a batch carrying both an
// already-present row and a new one inserts only the new one, and that the
// existing row is left untouched (ON CONFLICT DO NOTHING, not DO UPDATE).
func TestStore_InsertBatch_MixedNewAndExisting(t *testing.T) {
	ctx, store := setup(t)

	existing := samplePayment(1)
	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{existing}, nil, 50))

	changed := samplePayment(1) // same PK as existing, but mutated payload
	changed.Facilitator = "0xCHANGED"
	fresh := samplePayment(2)

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{changed, fresh}, nil, 100))

	require.Equal(t, int64(2), countPayments(ctx, t, store), "only the new row was added")

	keptOriginal := readPayment(ctx, t, store, existing.TxHash, 1)
	require.Equal(t, "0xfac", keptOriginal.Facilitator, "existing row untouched by DO NOTHING")

	addedFresh := readPayment(ctx, t, store, fresh.TxHash, 2)
	require.Equal(t, "0xfac", addedFresh.Facilitator)

	cursor, err := store.GetCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), cursor)
}
