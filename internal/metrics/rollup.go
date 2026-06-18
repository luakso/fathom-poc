package metrics

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rebuildCubeSQL recomputes the cube in one pass. day is the UTC calendar day.
// membership comes from the x402 view's facilitator_known flag; amount_band from
// the migration's IMMUTABLE function. TRUNCATE + INSERT makes the operation
// idempotent and safe to re-run after any backfill.
const rebuildCubeSQL = `
TRUNCATE metrics_daily_v2;
INSERT INTO metrics_daily_v2
    (day, chain, facilitator, membership, amount_band, methodology_version, txn_count, volume_usdc, max_amount_usdc)
SELECT
    (block_timestamp AT TIME ZONE 'UTC')::date AS day,
    chain,
    facilitator,
    CASE WHEN facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
    amount_band(amount_usdc) AS amount_band,
    methodology_version,
    count(*)                  AS txn_count,
    sum(amount_usdc)          AS volume_usdc,
    max(amount_usdc)          AS max_amount_usdc
FROM payment_x402_v1
GROUP BY 1, 2, 3, 4, 5, 6`

// tempFileLimit caps spill-to-disk for the percentile/CROSS JOIN statements in
// this registry. This deliberately overrides the server default upward for the
// offline rebuild (superuser-only GUC — the publisher connects as the bootstrap
// superuser; under a least-privilege role this SET fails).
const tempFileLimit = "30GB"

// missingPriceMonthsSQL lists months present in payments but absent from the
// session's eth_price_monthly temp table. NULL result = full coverage.
const missingPriceMonthsSQL = `
SELECT string_agg(m, ', ' ORDER BY m) FROM (
    SELECT DISTINCT to_char(block_timestamp AT TIME ZONE 'UTC', 'YYYY-MM') AS m FROM payments
    EXCEPT
    SELECT month FROM eth_price_monthly
) missing`

// windowsValues enumerates the fixed emit windows for rollup-side anchoring.
// days=0 means no lower bound ('all'). Anchored to max(day) of the data — the
// same anchor emit uses, so rollup and emit always agree on what '7d' means.
const windowsValues = `(VALUES ('7d', 7), ('30d', 30), ('all', 0)) AS w(window_name, days)`

// economyWindowStatsSQL: medians per (window, membership) + the synthetic
// 'all' membership via GROUPING SETS. Medians are not mergeable from a cube,
// so they are computed here, once, at scan time. min(methodology_version) is
// safe only because the v1 view is frozen single-version; emit independently
// asserts exactly one version across all metrics tables.
const economyWindowStatsSQL = `
TRUNCATE metrics_window_stats_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
)
INSERT INTO metrics_window_stats_v2
    (window_name, membership, methodology_version, txn_count, median_amount_usdc)
SELECT
    w.window_name,
    COALESCE(CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END, 'all'),
    min(p.methodology_version),
    count(*),
    percentile_disc(0.5) WITHIN GROUP (ORDER BY p.amount_usdc)
FROM payment_x402_v1 p
CROSS JOIN ` + windowsValues + `
CROSS JOIN anchor a
WHERE w.days = 0
   OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
GROUP BY w.window_name, GROUPING SETS ((CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END), ())`

// economyPricePointsSQL: top 50 exact known-facilitator amounts per window,
// ranked by txn count (ties broken by amount for determinism). payee_count
// separates menu from market. See economyWindowStatsSQL for the
// min(methodology_version) note: safe only because the v1 view is frozen
// single-version.
const economyPricePointsSQL = `
TRUNCATE metrics_price_points_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
)
INSERT INTO metrics_price_points_v2
    (window_name, rank, amount_usdc, txn_count, volume_usdc, payee_count, methodology_version)
SELECT window_name, rk, amount_usdc, txn_count, volume_usdc, payee_count, methodology_version
FROM (
    SELECT
        w.window_name,
        row_number() OVER (
            PARTITION BY w.window_name
            ORDER BY count(*) DESC, p.amount_usdc
        ) AS rk,
        p.amount_usdc,
        count(*)                 AS txn_count,
        sum(p.amount_usdc)       AS volume_usdc,
        count(DISTINCT p.payee)  AS payee_count,
        min(p.methodology_version) AS methodology_version
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE p.facilitator_known
      AND (w.days = 0
           OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1))
    GROUP BY w.window_name, p.amount_usdc
) ranked
WHERE rk <= 50`

// economyGasDailySQL: settlement cost at (day, membership, band) grain. Both
// gas_cost_wei (L2 execution) and l1_fee (L1 data fee) in payments are TX-level,
// duplicated onto every row of a batch — so each is deduped per (chain, tx_hash)
// (max() over identical values; l1_fee is NULLABLE so COALESCE to 0) and
// apportioned tx_cost/n equally across the tx's payments. The two components are
// stored separately (l2_gas_cost_wei, l1_fee_wei); cost_usd and breakeven use the
// TOTAL (l2 + l1). The apportioned sum conserves the per-tx sum to ~16 significant
// figures (numeric division; sub-wei drift, far below the 6dp ETH any artifact
// reports). USD uses the staged monthly reference price; breakeven counts payments
// whose apportioned total cost in USD exceeds the amount they moved. Membership is
// tx-level by construction, so apportioning never crosses membership.
const economyGasDailySQL = `
TRUNCATE metrics_gas_daily_v2;
WITH tx AS (
    SELECT chain, tx_hash,
           count(*) AS n,
           max(gas_cost_wei)        AS tx_l2,
           max(COALESCE(l1_fee, 0)) AS tx_l1
    FROM payment_x402_v1
    GROUP BY chain, tx_hash
)
INSERT INTO metrics_gas_daily_v2
    (day, membership, amount_band, methodology_version,
     txn_count, l2_gas_cost_wei, l1_fee_wei, cost_usd, breakeven_txn_count, volume_usdc)
SELECT
    (p.block_timestamp AT TIME ZONE 'UTC')::date AS day,
    CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
    amount_band(p.amount_usdc) AS amount_band,
    min(p.methodology_version),
    count(*),
    sum(t.tx_l2 / t.n),
    sum(t.tx_l1 / t.n),
    sum((t.tx_l2 + t.tx_l1) / t.n * pr.usd / '1000000000000000000'::numeric),
    count(*) FILTER (
        WHERE (t.tx_l2 + t.tx_l1) / t.n * pr.usd / '1000000000000000000'::numeric > p.amount_usdc),
    sum(p.amount_usdc)
FROM payment_x402_v1 p
JOIN tx t USING (chain, tx_hash)
JOIN eth_price_monthly pr
  ON pr.month = to_char(p.block_timestamp AT TIME ZONE 'UTC', 'YYYY-MM')
GROUP BY 1, 2, 3`

// economyVelocityDailySQL: per-minute counts reduced to per-day stats.
// p99 is over the day's ACTIVE minutes (idle minutes would zero it).
const economyVelocityDailySQL = `
TRUNCATE metrics_velocity_daily_v2;
INSERT INTO metrics_velocity_daily_v2
    (day, membership, methodology_version, txn_count, max_per_min, p99_per_min)
SELECT
    day, membership, min(mv), sum(c), max(c),
    percentile_disc(0.99) WITHIN GROUP (ORDER BY c)
FROM (
    SELECT
        (block_timestamp AT TIME ZONE 'UTC')::date AS day,
        CASE WHEN facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
        date_trunc('minute', block_timestamp AT TIME ZONE 'UTC') AS minute,
        min(methodology_version) AS mv,
        count(*) AS c
    FROM payment_x402_v1
    GROUP BY 1, 2, 3
) per_min
GROUP BY day, membership`

// rebuildStatements run in order inside the one rebuild transaction. Each is
// its own TRUNCATE + INSERT, so a failure anywhere rolls the whole generation
// back and the previous tables stay live.
var rebuildStatements = []struct {
	name string
	sql  string
}{
	{"cube", rebuildCubeSQL},
	{"window_stats", economyWindowStatsSQL},
	{"price_points", economyPricePointsSQL},
	{"gas_daily", economyGasDailySQL},
	{"velocity_daily", economyVelocityDailySQL},
}

// Rebuild fully recomputes every metrics table from payment_x402_v1 in a
// single transaction. prices is the curated monthly ETH/USD reference (already
// validated by LoadETHPrices); it is staged into a temp table so the gas SQL
// can join it, and coverage is checked against payments BEFORE any TRUNCATE.
func Rebuild(ctx context.Context, pool *pgxpool.Pool, prices ETHPrices) error {
	// REPEATABLE READ: every statement (and its per-statement anchor CTE) sees
	// one snapshot, so the window-grain tables and the cube can never anchor to
	// different days — the same hardening Emit uses on the read side.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return fmt.Errorf("begin rebuild tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Guard against runaway spill from the percentile/CROSS JOIN statements in
	// the rebuild registry.
	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL temp_file_limit = '%s'`, tempFileLimit)); err != nil {
		return fmt.Errorf("set temp_file_limit: %w", err)
	}

	if err := stagePrices(ctx, tx, prices); err != nil {
		return err
	}

	var missing *string
	if err := tx.QueryRow(ctx, missingPriceMonthsSQL).Scan(&missing); err != nil {
		return fmt.Errorf("check price coverage: %w", err)
	}
	if missing != nil {
		return fmt.Errorf("eth price file is missing months present in payments: %s", *missing)
	}

	for _, stmt := range rebuildStatements {
		if _, err := tx.Exec(ctx, stmt.sql); err != nil {
			return fmt.Errorf("rebuild %s: %w", stmt.name, err)
		}
	}
	if err := RebuildEntities(ctx, tx); err != nil {
		return fmt.Errorf("rebuild entities: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rebuild: %w", err)
	}
	return nil
}

// stagePrices creates the session-scoped monthly price table the gas SQL joins.
func stagePrices(ctx context.Context, tx pgx.Tx, prices ETHPrices) error {
	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE eth_price_monthly (
			month TEXT PRIMARY KEY,
			usd   NUMERIC NOT NULL CHECK (usd > 0)
		) ON COMMIT DROP`); err != nil {
		return fmt.Errorf("create price temp table: %w", err)
	}
	months := make([]string, 0, len(prices.Prices))
	for m := range prices.Prices {
		months = append(months, m)
	}
	sort.Strings(months)
	for _, m := range months {
		if _, err := tx.Exec(ctx,
			`INSERT INTO eth_price_monthly (month, usd) VALUES ($1, $2)`,
			m, prices.Prices[m].String()); err != nil {
			return fmt.Errorf("stage price %s: %w", m, err)
		}
	}
	return nil
}
