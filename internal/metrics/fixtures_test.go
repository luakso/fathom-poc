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

// testPrices covers every month a fixture could reasonably seed (2025-11
// through 2026-12) at a flat $2000/ETH, so gas math in tests is easy to
// compute by hand: 1e15 wei ≈ $2.
func testPrices(t *testing.T) metrics.ETHPrices {
	t.Helper()
	prices := map[string]decimal.Decimal{}
	for _, m := range []string{
		"2025-11", "2025-12", "2026-01", "2026-02", "2026-03", "2026-04",
		"2026-05", "2026-06", "2026-07", "2026-08", "2026-09", "2026-10",
		"2026-11", "2026-12",
	} {
		prices[m] = decimal.NewFromInt(2000)
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
