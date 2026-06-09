//go:build integration

package metrics_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
