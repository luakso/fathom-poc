//go:build integration

package anatomy_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

// seedPaymentAt inserts one payment with an explicit timestamp (seedPayment in
// dossier_test.go pins 2026-06-01; window tests need spread-out days).
func seedPaymentAt(t *testing.T, ctx context.Context, db *sql.DB,
	txHash string, logIdx int, ts, payer, facilitator, payee, amt string,
) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO payments (
			chain, tx_hash, log_index, block_number, block_timestamp, source, protocol,
			facilitator, payer, payee, asset, token_address, amount_raw,
			asset_usd_at_time, auth_nonce, method_selector, called_contract, tx_type,
			tx_nonce, gas_used, effective_gas_price, gas_cost_wei
		) VALUES (
			'base', $1, $2, 100, $3, 'test', 'x402',
			$4, $5, $6, 'USDC', '0xtoken', ($7::numeric * 1000000)::numeric(78,0),
			1.0, '\x01', '\xdeadbeef', '0xcontract', 2,
			7, 50000, 1000000, 50000000
		)`, txHash, logIdx, ts, facilitator, payer, payee, amt)
	require.NoError(t, err)
}

// allowFacilitator marks an address facilitator_known under methodology v1.
func allowFacilitator(t *testing.T, ctx context.Context, db *sql.DB, addr string) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO facilitator_allowlist (chain, address, source, label, since_version)
		VALUES ('base', $1, 'manual', 'KnownFac', 1)
		ON CONFLICT DO NOTHING`, addr)
	require.NoError(t, err)
}

// seedRollupFixture: 2 facilitators (one known), 2 payers, 2 payees, 5 payments
// across 3 days. Returns nothing; tests assert against known totals.
//
//	day 2026-06-01: p1 -fk-> e1 $2.00 ; p1 -fk-> e2 $3.00
//	day 2026-06-02: p1 -fk-> e1 $5.00 ; p2 -fu-> e1 $7.00
//	day 2026-06-03: p2 -fu-> e2 $11.00
//
// fk = known facilitator 0xkfac, fu = unknown facilitator 0xufac.
func seedRollupFixture(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	allowFacilitator(t, ctx, db, "0xkfac")
	seedPaymentAt(t, ctx, db, "0xt1", 0, "2026-06-01T08:00:00Z", "0xp1", "0xkfac", "0xe1", "2.00")
	seedPaymentAt(t, ctx, db, "0xt2", 0, "2026-06-01T09:00:00Z", "0xp1", "0xkfac", "0xe2", "3.00")
	seedPaymentAt(t, ctx, db, "0xt3", 0, "2026-06-02T08:00:00Z", "0xp1", "0xkfac", "0xe1", "5.00")
	seedPaymentAt(t, ctx, db, "0xt4", 0, "2026-06-02T09:00:00Z", "0xp2", "0xufac", "0xe1", "7.00")
	seedPaymentAt(t, ctx, db, "0xt5", 0, "2026-06-03T08:00:00Z", "0xp2", "0xufac", "0xe2", "11.00")
}

// sumQuery scans a (count, volume) pair from any aggregate query.
func sumQuery(t *testing.T, ctx context.Context, db *sql.DB, q string, args ...any) (int64, string) {
	t.Helper()
	var n int64
	var vol string
	require.NoError(t, db.QueryRowContext(ctx, q, args...).Scan(&n, &vol))
	return n, vol
}

func TestRollup_EdgeAndDayConservation(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)

	require.NoError(t, anatomy.Rollup(ctx, pool, nil))

	// Conservation: entity_day payer-role totals == view totals, per lens.
	for _, tc := range []struct {
		known  bool
		txns   int64
		volume string
	}{
		{true, 3, "10.000000"},  // t1+t2+t3
		{false, 2, "18.000000"}, // t4+t5
	} {
		n, vol := sumQuery(t, ctx, db, `
			SELECT COALESCE(sum(txn_count),0), COALESCE(sum(volume_usdc),0)::text
			FROM entity_day_v1 WHERE role = 'payer' AND facilitator_known = $1`, tc.known)
		require.Equal(t, tc.txns, n, "payer day txns known=%v", tc.known)
		require.Equal(t, tc.volume, vol, "payer day volume known=%v", tc.known)
	}

	// Each role slice carries every payment exactly once.
	for _, role := range []string{"payer", "payee", "facilitator"} {
		n, vol := sumQuery(t, ctx, db, `
			SELECT COALESCE(sum(txn_count),0), COALESCE(sum(volume_usdc),0)::text
			FROM entity_day_v1 WHERE role = $1`, role)
		require.Equal(t, int64(5), n, "role %s total txns", role)
		require.Equal(t, "28.000000", vol, "role %s total volume", role)
	}

	// Edge totals == day totals.
	n, vol := sumQuery(t, ctx, db, `
		SELECT COALESCE(sum(txn_count),0), COALESCE(sum(volume_usdc),0)::text
		FROM entity_edge_v1`)
	require.Equal(t, int64(5), n)
	require.Equal(t, "28.000000", vol)

	// A concrete edge: p1 -> e1 under the known lens (t1 + t3).
	n, vol = sumQuery(t, ctx, db, `
		SELECT txn_count, volume_usdc::text FROM entity_edge_v1
		WHERE chain='base' AND payer='0xp1' AND payee='0xe1' AND facilitator_known`)
	require.Equal(t, int64(2), n)
	require.Equal(t, "7.000000", vol)

	// first/last seen on that edge span t1..t3.
	var first, last string
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT first_seen::text, last_seen::text FROM entity_edge_v1
		WHERE payer='0xp1' AND payee='0xe1' AND facilitator_known`).Scan(&first, &last))
	require.Contains(t, first, "2026-06-01")
	require.Contains(t, last, "2026-06-02")

	// Facilitator edges: each payment contributes one payer-side and one
	// payee-side row set, so each side conserves the totals.
	for _, side := range []string{"payer", "payee"} {
		n, vol = sumQuery(t, ctx, db, `
			SELECT COALESCE(sum(txn_count),0), COALESCE(sum(volume_usdc),0)::text
			FROM facilitator_edge_v1 WHERE counterparty_role = $1`, side)
		require.Equal(t, int64(5), n, "facilitator edge %s txns", side)
		require.Equal(t, "28.000000", vol, "facilitator edge %s volume", side)
	}

	// Known facilitator serves exactly p1 on the payer side.
	n, vol = sumQuery(t, ctx, db, `
		SELECT txn_count, volume_usdc::text FROM facilitator_edge_v1
		WHERE facilitator='0xkfac' AND counterparty_role='payer' AND counterparty='0xp1'`)
	require.Equal(t, int64(3), n)
	require.Equal(t, "10.000000", vol)
}
