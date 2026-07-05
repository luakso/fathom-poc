//go:build integration

package metrics_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

// seedPayments inserts rows straight into the payments table for tests. Only the
// columns the cube reads are meaningful; the rest get valid placeholder values so
// the NOT NULL constraints pass. ts is an ISO timestamp; amountUSDC is decimal text.
//
// amount_usdc is NOT inserted: migration 00008 makes it a GENERATED column
// (amount_raw * 0.000001). We set amount_raw = amountUSDC * 1e6 so the generated
// amount_usdc equals the requested value. amountUSDC values in fixtures must have
// at most 6 decimal places (so amount_raw stays an integer / scale 0).
type seedRow struct {
	txHash      string
	logIndex    int
	ts          string // e.g. '2026-06-01T10:00:00Z'
	facilitator string
	payer       string
	payee       string
	amountUSDC  string
}

func seedPayments(t *testing.T, ctx context.Context, db *sql.DB, rows []seedRow) {
	t.Helper()
	for _, r := range rows {
		_, err := db.ExecContext(ctx, `
			INSERT INTO payments (
				chain, tx_hash, log_index, block_number, block_timestamp,
				source, protocol, facilitator, payer, payee,
				asset, token_address, amount_raw, asset_usd_at_time,
				auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
				gas_used, effective_gas_price, gas_cost_wei
			) VALUES (
				'base', $1, $2, 1, $3::timestamptz,
				'test', 'x402', $4, $5, $6,
				'USDC', '0xusdc', ($7::numeric * 1000000)::numeric(78,0), 1,
				'\x00', '\x12345678', '0xvenue', 2, 0,
				0, 0, 0
			)`,
			r.txHash, r.logIndex, r.ts, r.facilitator, r.payer, r.payee, r.amountUSDC)
		require.NoError(t, err)
	}
}

// allowlist marks a facilitator address as agentic at v1 (so the view labels its
// rows 'agentic' instead of 'contested').
func allowlist(t *testing.T, ctx context.Context, db *sql.DB, addr string) {
	t.Helper()
	_, err := db.ExecContext(ctx,
		`INSERT INTO facilitator_allowlist (chain, address, source, since_version) VALUES ('base',$1,'test',1)`, addr)
	require.NoError(t, err)
}

func mustTime(t *testing.T, iso string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, iso)
	require.NoError(t, err)
	return ts
}

// testPrices covers every ISO week-start (Monday) a fixture could reasonably
// seed (2025-10-06 through 2026-12-28) at a flat $2000/ETH, so gas math in
// tests is easy to compute by hand: 1e15 wei ≈ $2.
func testPrices(t *testing.T) metrics.ETHPrices {
	t.Helper()
	prices := map[string]decimal.Decimal{}
	// 2025-10-06 is a Monday; iterate weekly through 2026-12-28.
	start := time.Date(2025, 10, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 7) {
		prices[d.Format("2006-01-02")] = decimal.NewFromInt(2000)
	}
	return metrics.ETHPrices{Source: "test", Unit: "USD per ETH", Prices: prices}
}

// seedGasRow seeds a payment with an explicit tx-level gas cost. Rows sharing
// a txHash form a batch; production data carries the IDENTICAL tx-level
// gas_cost_wei on every row of a batch, and these fixtures must mirror that.
type seedGasRow struct {
	txHash      string
	logIndex    int
	ts          string
	facilitator string
	payer       string
	payee       string
	amountUSDC  string
	gasCostWei  string // tx-level wei, repeated on each row of the batch
}

func seedGasPayments(t *testing.T, ctx context.Context, db *sql.DB, rows []seedGasRow) {
	t.Helper()
	for _, r := range rows {
		_, err := db.ExecContext(ctx, `
			INSERT INTO payments (
				chain, tx_hash, log_index, block_number, block_timestamp,
				source, protocol, facilitator, payer, payee,
				asset, token_address, amount_raw, asset_usd_at_time,
				auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
				gas_used, effective_gas_price, gas_cost_wei
			) VALUES (
				'base', $1, $2, 1, $3::timestamptz,
				'test', 'x402', $4, $5, $6,
				'USDC', '0xusdc', ($7::numeric * 1000000)::numeric(78,0), 1,
				'\x00', '\x12345678', '0xvenue', 2, 0,
				21000, 1, $8::numeric
			)`,
			r.txHash, r.logIndex, r.ts, r.facilitator, r.payer, r.payee, r.amountUSDC, r.gasCostWei)
		require.NoError(t, err)
	}
}

// seedL1GasRow seeds a payment with explicit tx-level L2 gas AND L1 data fee,
// both carried identically on every row of a batch (mirrors production capture).
type seedL1GasRow struct {
	txHash      string
	logIndex    int
	ts          string
	facilitator string
	payer       string
	payee       string
	amountUSDC  string
	gasCostWei  string // tx-level L2 execution gas
	l1FeeWei    string // tx-level L1 data fee
}

func seedL1GasPayments(t *testing.T, ctx context.Context, db *sql.DB, rows []seedL1GasRow) {
	t.Helper()
	for _, r := range rows {
		_, err := db.ExecContext(ctx, `
			INSERT INTO payments (
				chain, tx_hash, log_index, block_number, block_timestamp,
				source, protocol, facilitator, payer, payee,
				asset, token_address, amount_raw, asset_usd_at_time,
				auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
				gas_used, effective_gas_price, gas_cost_wei, l1_fee
			) VALUES (
				'base', $1, $2, 1, $3::timestamptz,
				'test', 'x402', $4, $5, $6,
				'USDC', '0xusdc', ($7::numeric * 1000000)::numeric(78,0), 1,
				'\x00', '\x12345678', '0xvenue', 2, 0,
				21000, 1, $8::numeric, $9::numeric
			)`,
			r.txHash, r.logIndex, r.ts, r.facilitator, r.payer, r.payee, r.amountUSDC, r.gasCostWei, r.l1FeeWei)
		require.NoError(t, err)
	}
}

// seedWindowedRow is a payment carrying an explicit EIP-3009 auth window, used by
// the reliability rollup tests. validAfter/validBefore are ISO timestamps or ""
// (→ NULL, the window-less batch case). Other columns mirror seedPayments.
type seedWindowedRow struct {
	txHash      string
	logIndex    int
	ts          string // block_timestamp
	facilitator string
	payer       string
	payee       string
	amountUSDC  string
	validAfter  string // "" → NULL
	validBefore string // "" → NULL
}

func seedWindowedPayments(t *testing.T, ctx context.Context, db *sql.DB, rows []seedWindowedRow) {
	t.Helper()
	for _, r := range rows {
		var va, vb any
		if r.validAfter != "" {
			va = r.validAfter
		}
		if r.validBefore != "" {
			vb = r.validBefore
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO payments (
				chain, tx_hash, log_index, block_number, block_timestamp,
				source, protocol, facilitator, payer, payee,
				asset, token_address, amount_raw, asset_usd_at_time,
				auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
				gas_used, effective_gas_price, gas_cost_wei,
				valid_after, valid_before
			) VALUES (
				'base', $1, $2, 1, $3::timestamptz,
				'test', 'x402', $4, $5, $6,
				'USDC', '0xusdc', ($7::numeric * 1000000)::numeric(78,0), 1,
				'\x00', '\x12345678', '0xvenue', 2, 0,
				0, 0, 0,
				$8::timestamptz, $9::timestamptz
			)`,
			r.txHash, r.logIndex, r.ts, r.facilitator, r.payer, r.payee, r.amountUSDC, va, vb)
		require.NoError(t, err)
	}
}

// seedMechRow exposes the columns the mechanics rollup reads. Empty strings map to
// NULL (max fee/priority, valid bounds). authNonceHex / selectorHex are bytea
// literals like '\\x1234'; gasUsed/gasLimit/txValue are numeric/bigint text.
type seedMechRow struct {
	txHash         string
	logIndex       int
	ts             string
	facilitator    string
	payer          string
	payee          string
	amountUSDC     string
	txType         int
	maxFee         string // wei text or "" → NULL
	maxPriority    string // wei text or "" → NULL
	gasUsed        string // bigint text
	gasLimit       string // bigint text or "" → NULL
	txValue        string // wei text (default "0")
	selectorHex    string // e.g. '\\x12345678'
	settlementKind string // 'transfer' | 'receive'
	authNonceHex   string // e.g. '\\xaa'
	blockNumber    int
	validAfter     string // "" → NULL
	validBefore    string // "" → NULL
}

func seedMechanicsPayments(t *testing.T, ctx context.Context, db *sql.DB, rows []seedMechRow) {
	t.Helper()
	for _, r := range rows {
		var mf, mp, va, vb, gl any
		if r.maxFee != "" {
			mf = r.maxFee
		}
		if r.maxPriority != "" {
			mp = r.maxPriority
		}
		if r.validAfter != "" {
			va = r.validAfter
		}
		if r.validBefore != "" {
			vb = r.validBefore
		}
		if r.gasLimit != "" {
			gl = r.gasLimit
		}
		txValue := r.txValue
		if txValue == "" {
			txValue = "0"
		}
		_, err := db.ExecContext(ctx, `
			INSERT INTO payments (
				chain, tx_hash, log_index, block_number, block_timestamp,
				source, protocol, facilitator, payer, payee,
				asset, token_address, amount_raw, asset_usd_at_time,
				auth_nonce, method_selector, called_contract, tx_type, tx_nonce,
				gas_used, effective_gas_price, gas_cost_wei,
				max_fee_per_gas, max_priority_fee_per_gas,
				settlement_kind, valid_after, valid_before,
				tx_value, gas_limit
			) VALUES (
				'base', $1, $2, $3, $4::timestamptz,
				'test', 'x402', $5, $6, $7,
				'USDC', '0xusdc', ($8::numeric * 1000000)::numeric(78,0), 1,
				$9::bytea, $10::bytea, '0xvenue', $11, 0,
				$12::numeric, 0, 0,
				$13::numeric, $14::numeric,
				$15, $16::timestamptz, $17::timestamptz,
				$18::numeric, $19::bigint
			)`,
			r.txHash, r.logIndex, r.blockNumber, r.ts, r.facilitator, r.payer, r.payee, r.amountUSDC,
			r.authNonceHex, r.selectorHex, r.txType, r.gasUsed, mf, mp, r.settlementKind, va, vb, txValue, gl)
		require.NoError(t, err)
	}
}

// seedCancelRow is one AuthorizationCanceled event for the reliability tests.
type seedCancelRow struct {
	txHash          string
	logIndex        int
	authorizer      string // the payer who signed then canceled
	blockTime       string // ISO timestamp
	transactionFrom string // cancel submitter (allowlisted → facilitator_known)
}

func seedCancellations(t *testing.T, ctx context.Context, db *sql.DB, rows []seedCancelRow) {
	t.Helper()
	for _, r := range rows {
		_, err := db.ExecContext(ctx, `
			INSERT INTO authorization_cancellations (
				chain, tx_hash, log_index, authorizer, nonce,
				block_number, block_time, transaction_from
			) VALUES ('base', $1, $2, $3, '\x00', 1, $4::timestamptz, $5)`,
			r.txHash, r.logIndex, r.authorizer, r.blockTime, r.transactionFrom)
		require.NoError(t, err)
	}
}
