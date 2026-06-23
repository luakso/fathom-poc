//go:build integration

package metrics_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/metrics"
)

func TestRebuildMechanics_Window(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1") // known
	seedMechanicsPayments(t, ctx, db, []seedMechRow{
		// type-2, over-provisioned (gas_used 21000 / gas_limit 42000 = 0.5), 1h width, tx_value 0
		{"0xa", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xaa`, 100, "2026-06-10T09:00:00Z", "2026-06-10T10:00:00Z"},
		// type-0, tx_value > 0 (canary), no max fee, no window
		{"0xb", 0, "2026-06-10T11:00:00Z", "0xfac1", "0xp2", "0xs1", "2.00", 0, "", "", "21000", "21000", "5", `\x11111111`, "transfer", `\xbb`, 101, "", ""},
		// unknown facilitator, type-2
		{"0xc", 0, "2026-06-10T12:00:00Z", "0xfac2", "0xp3", "0xs2", "3.00", 2, "2000", "200", "30000", "60000", "0", `\x22222222`, "receive", `\xcc`, 102, "2026-06-10T08:00:00Z", "2026-06-10T10:00:00Z"},
		// DUPLICATE (payer,auth_nonce) of 0xa → M12 dup canary (same payer 0xp1, same nonce \xaa)
		{"0xd", 0, "2026-06-10T13:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xaa`, 103, "2026-06-10T09:00:00Z", "2026-06-10T10:00:00Z"},
	})

	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	var settle, t0, t2, txvNonzero, dup, widthCount int64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT settlement_count, tx_type_0_count, tx_type_2_count, tx_value_nonzero_count, dup_auth_nonce_count, width_count
		FROM metrics_mechanics_window_v2 WHERE window_name='all' AND membership='all'`).
		Scan(&settle, &t0, &t2, &txvNonzero, &dup, &widthCount))
	require.Equal(t, int64(4), settle)
	require.Equal(t, int64(1), t0, "one type-0 (0xb)")
	require.Equal(t, int64(3), t2, "three type-2")
	require.Equal(t, int64(1), txvNonzero, "0xb carries tx_value>0")
	require.Equal(t, int64(1), dup, "0xd duplicates (0xp1,\\xaa)")
	require.Equal(t, int64(3), widthCount, "three carry both auth bounds (0xb has none)")

	// over-provisioning ratio p50: known rows with gas_limit are 0xa,0xd (0.5,0.5) and 0xb (1.0) → median 0.5
	var ratioP50 float64
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT overprov_ratio_p50 FROM metrics_mechanics_window_v2 WHERE window_name='all' AND membership='known'`).Scan(&ratioP50))
	require.InDelta(t, 0.5, ratioP50, 1e-9)

	// membership reconciliation
	var known, unknown int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_mechanics_window_v2 WHERE window_name='all' AND membership='known'`).Scan(&known))
	require.NoError(t, db.QueryRowContext(ctx, `SELECT settlement_count FROM metrics_mechanics_window_v2 WHERE window_name='all' AND membership='unknown'`).Scan(&unknown))
	require.Equal(t, settle, known+unknown)
	require.Equal(t, int64(3), known)
	require.Equal(t, int64(1), unknown)
}

func TestRebuildMechanics_BatchAndBlock(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// tx 0xbatch has 3 payments (batch size 3); tx 0xsingle has 1. Two payments share block 200.
	seedMechanicsPayments(t, ctx, db, []seedMechRow{
		{"0xbatch", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\x01`, 200, "", ""},
		{"0xbatch", 1, "2026-06-10T10:00:00Z", "0xfac1", "0xp2", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\x02`, 200, "", ""},
		{"0xbatch", 2, "2026-06-10T10:00:00Z", "0xfac1", "0xp3", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\x03`, 200, "", ""},
		{"0xsingle", 0, "2026-06-10T11:00:00Z", "0xfac1", "0xp4", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\x04`, 201, "", ""},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// batch: bucket '1' has 1 tx / 1 payment; bucket '2-10' has 1 tx / 3 payments; max 3.
	var tx1, pay1, max1 int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT tx_count, payment_count, max_batch_size FROM metrics_mechanics_batch_v2 WHERE window_name='all' AND batch_bucket='1'`).Scan(&tx1, &pay1, &max1))
	require.Equal(t, int64(1), tx1)
	require.Equal(t, int64(1), pay1)
	require.Equal(t, int64(3), max1)
	var tx210, pay210 int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT tx_count, payment_count FROM metrics_mechanics_batch_v2 WHERE window_name='all' AND batch_bucket='2-10'`).Scan(&tx210, &pay210))
	require.Equal(t, int64(1), tx210)
	require.Equal(t, int64(3), pay210)
	// payment_count conserves to settlement_count (4).
	var paySum int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT sum(payment_count) FROM metrics_mechanics_batch_v2 WHERE window_name='all'`).Scan(&paySum))
	require.Equal(t, int64(4), paySum)

	// block: block 200 has 3 payments, 201 has 1 → max_per_block 3, distinct_blocks 2.
	var maxBlk, distinct int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT max_per_block, distinct_blocks FROM metrics_mechanics_block_v2 WHERE window_name='all'`).Scan(&maxBlk, &distinct))
	require.Equal(t, int64(3), maxBlk)
	require.Equal(t, int64(2), distinct)
}

func TestRebuildMechanics_SelectorMix(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	// selector 11111111 transfer x3, selector 22222222 receive x1 — all known.
	seedMechanicsPayments(t, ctx, db, []seedMechRow{
		{"0xa", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xa1`, 300, "", ""},
		{"0xb", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp2", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xa2`, 301, "", ""},
		{"0xc", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp3", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xa3`, 302, "", ""},
		{"0xd", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp4", "0xs1", "9.00", 2, "1000", "100", "21000", "42000", "0", `\x22222222`, "receive", `\xa4`, 303, "", ""},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	// rank 1 known = selector 11111111/transfer with 3 txns; conservation: txns sum to 4.
	var hex, kind string
	var txns int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT selector_hex, settlement_kind, txn_count FROM metrics_mechanics_selector_v2 WHERE window_name='all' AND membership='known' AND rank=1`).Scan(&hex, &kind, &txns))
	require.Equal(t, "11111111", hex)
	require.Equal(t, "transfer", kind)
	require.Equal(t, int64(3), txns)
	var txnSum int64
	require.NoError(t, db.QueryRowContext(ctx, `SELECT sum(txn_count) FROM metrics_mechanics_selector_v2 WHERE window_name='all' AND membership='all'`).Scan(&txnSum))
	require.Equal(t, int64(4), txnSum, "selector mix conserves to settlements in the all-membership slice")
}

func TestBuildMechanics_Shape(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedMechanicsPayments(t, ctx, db, []seedMechRow{
		{"0xa", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xa1`, 400, "2026-06-10T09:00:00Z", "2026-06-10T10:00:00Z"},
		{"0xb", 0, "2026-06-10T10:00:00Z", "0xfac2", "0xp2", "0xs1", "2.00", 0, "", "", "21000", "21000", "0", `\x11111111`, "transfer", `\xa2`, 400, "", ""},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	page, err := metrics.BuildMechanics(ctx, pool)
	require.NoError(t, err)
	all := page.Windows["all"]
	require.Equal(t, int64(2), all.SettlementCount)
	require.Equal(t, int64(1), all.Fee.TxType["0"])
	require.Equal(t, int64(1), all.Fee.TxType["2"])
	require.NotEmpty(t, all.SelectorMix)
	require.Equal(t, int64(2), all.BlockDensity.MaxPerBlock, "both in block 400")
	require.GreaterOrEqual(t, all.Cost.BreakevenTxnCount, int64(0)) // cost block populated from gas cube
	require.Contains(t, all.ByMembership, "known")
}

func TestEmit_WritesMechanics(t *testing.T) {
	ctx, db, pool := setupMetrics(t)
	allowlist(t, ctx, db, "0xfac1")
	seedMechanicsPayments(t, ctx, db, []seedMechRow{
		{"0xa", 0, "2026-06-10T10:00:00Z", "0xfac1", "0xp1", "0xs1", "1.00", 2, "1000", "100", "21000", "42000", "0", `\x11111111`, "transfer", `\xa1`, 500, "2026-06-10T09:00:00Z", "2026-06-10T10:00:00Z"},
		{"0xb", 0, "2026-06-10T10:00:00Z", "0xfac2", "0xp2", "0xs1", "2.00", 0, "", "", "21000", "21000", "0", `\x11111111`, "transfer", `\xa2`, 500, "", ""},
	})
	require.NoError(t, metrics.Rebuild(ctx, pool, testPrices(t)))

	dir := t.TempDir()
	require.NoError(t, metrics.Emit(ctx, pool, dir, nil))

	b, err := os.ReadFile(filepath.Join(dir, "mechanics.json"))
	require.NoError(t, err)
	var doc struct {
		Data struct {
			Windows map[string]struct {
				SettlementCount int64 `json:"settlement_count"`
				Fee             struct {
					TxType map[string]int64 `json:"tx_type"`
				} `json:"fee"`
				BlockDensity struct {
					MaxPerBlock int64 `json:"max_per_block"`
				} `json:"block_density"`
				SelectorMix  []map[string]any `json:"selector_mix"`
				ByMembership map[string]struct {
					SettlementCount int64 `json:"settlement_count"`
				} `json:"by_membership"`
			} `json:"windows"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &doc))
	all := doc.Data.Windows["all"]
	require.Equal(t, int64(2), all.SettlementCount)
	require.Equal(t, int64(2), all.BlockDensity.MaxPerBlock)
	require.NotEmpty(t, all.SelectorMix)
	require.Equal(t, all.SettlementCount,
		all.ByMembership["known"].SettlementCount+all.ByMembership["unknown"].SettlementCount)
}
