package metrics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// mechanicsWindowSQL: per-(window, membership) per-payment-grain mechanics stats.
// One scan of payment_x402_v1 over the windows CROSS JOIN. PERF: the three quantiles
// of each column are computed with the ARRAY form of percentile_cont in a single
// sort (4 sorts total, not 12), inside the `agg` CTE; the outer INSERT just indexes
// the arrays. M12 hygiene (the two composite count(DISTINCT)) is computed ONCE over
// the full table in a trailing UPDATE (global QA canary), NOT entangled in the
// per-membership x windows CROSS JOIN. 'all' membership via GROUPING SETS.
const mechanicsWindowSQL = `
TRUNCATE metrics_mechanics_window_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
),
base AS (
    SELECT
        w.window_name,
        CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
        p.methodology_version, p.tx_type,
        p.max_fee_per_gas, p.max_priority_fee_per_gas, p.tx_value,
        CASE WHEN p.valid_after IS NOT NULL AND p.valid_before IS NOT NULL
             THEN EXTRACT(EPOCH FROM (p.valid_before - p.valid_after)) END AS width_s,
        CASE WHEN p.gas_limit IS NOT NULL AND p.gas_limit > 0
             THEN p.gas_used::numeric / p.gas_limit END AS overprov_ratio
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE w.days = 0
       OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
),
agg AS (
    SELECT
        window_name, membership, min(methodology_version) AS mv, count(*) AS settle,
        count(*) FILTER (WHERE tx_type = 0) AS t0,
        count(*) FILTER (WHERE tx_type = 1) AS t1,
        count(*) FILTER (WHERE tx_type = 2) AS t2,
        count(*) FILTER (WHERE width_s IS NOT NULL) AS width_count,
        count(*) FILTER (WHERE overprov_ratio IS NOT NULL) AS overprov_count,
        count(*) FILTER (WHERE tx_value > 0) AS txv_nonzero,
        percentile_cont(ARRAY[0.5,0.9,0.99]) WITHIN GROUP (ORDER BY max_fee_per_gas) FILTER (WHERE max_fee_per_gas IS NOT NULL) AS mf,
        percentile_cont(ARRAY[0.5,0.9,0.99]) WITHIN GROUP (ORDER BY max_priority_fee_per_gas) FILTER (WHERE max_priority_fee_per_gas IS NOT NULL) AS mp,
        percentile_cont(ARRAY[0.5,0.9,0.99]) WITHIN GROUP (ORDER BY width_s) FILTER (WHERE width_s >= 0) AS wd,
        percentile_cont(ARRAY[0.5,0.9,0.99]) WITHIN GROUP (ORDER BY overprov_ratio) FILTER (WHERE overprov_ratio IS NOT NULL) AS op
    FROM base
    GROUP BY window_name, GROUPING SETS ((membership), ())
)
INSERT INTO metrics_mechanics_window_v2
    (window_name, membership, methodology_version, settlement_count,
     tx_type_0_count, tx_type_1_count, tx_type_2_count,
     max_fee_p50, max_fee_p90, max_fee_p99,
     max_priority_p50, max_priority_p90, max_priority_p99,
     width_count, width_p50_s, width_p90_s, width_p99_s,
     overprov_count, overprov_ratio_p50, overprov_ratio_p90, overprov_ratio_p99,
     tx_value_nonzero_count)
SELECT
    window_name, COALESCE(membership, 'all'), mv, settle,
    t0, t1, t2,
    mf[1], mf[2], mf[3],
    mp[1], mp[2], mp[3],
    width_count, wd[1], wd[2], wd[3],
    overprov_count, op[1], op[2], op[3],
    txv_nonzero
FROM agg;

-- M12 data hygiene: a GLOBAL QA canary computed once over the full table (one
-- scan, two composite count(DISTINCT)) and written to the all/all row only. Kept
-- out of the heavy per-membership x windows scan above.
UPDATE metrics_mechanics_window_v2 t
SET dup_auth_nonce_count = h.dup, same_block_replay_count = h.replay
FROM (
    SELECT
        count(*) - count(DISTINCT (payer, auth_nonce))               AS dup,
        count(*) - count(DISTINCT (payer, auth_nonce, block_number)) AS replay
    FROM payment_x402_v1
) h
WHERE t.window_name = 'all' AND t.membership = 'all';`

// mechanicsBatchSQL: M3 payments-per-tx histogram + per-window max (denormalized).
const mechanicsBatchSQL = `
TRUNCATE metrics_mechanics_batch_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
),
txn AS (
    SELECT w.window_name, p.tx_hash, count(*) AS n, min(p.methodology_version) AS mv
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE w.days = 0 OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
    GROUP BY w.window_name, p.tx_hash
),
maxes AS (SELECT window_name, max(n) AS max_n FROM txn GROUP BY window_name)
INSERT INTO metrics_mechanics_batch_v2
    (window_name, batch_bucket, methodology_version, tx_count, payment_count, max_batch_size)
SELECT t.window_name,
    CASE WHEN n = 1 THEN '1' WHEN n <= 10 THEN '2-10' WHEN n <= 100 THEN '11-100' ELSE '100+' END,
    min(t.mv), count(*), sum(t.n), m.max_n
FROM txn t JOIN maxes m USING (window_name)
GROUP BY t.window_name, 2, m.max_n;`

// mechanicsBlockSQL: M5 payments-per-block stats per window.
const mechanicsBlockSQL = `
TRUNCATE metrics_mechanics_block_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
),
blk AS (
    SELECT w.window_name, p.block_number, count(*) AS n, min(p.methodology_version) AS mv
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE w.days = 0 OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
    GROUP BY w.window_name, p.block_number
)
INSERT INTO metrics_mechanics_block_v2
    (window_name, methodology_version, max_per_block, p99_per_block, mean_per_block, distinct_blocks)
SELECT window_name, min(mv), max(n),
    percentile_disc(0.99) WITHIN GROUP (ORDER BY n),
    avg(n)::double precision, count(*)
FROM blk
GROUP BY window_name;`

// mechanicsSelectorSQL: M2 top-15 (selector_hex, settlement_kind) per
// (window, membership) by txn_count. 'all' membership via GROUPING SETS, ranked
// in an outer window function.
const mechanicsSelectorSQL = `
TRUNCATE metrics_mechanics_selector_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
),
agg AS (
    SELECT
        w.window_name,
        COALESCE(CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END, 'all') AS membership,
        encode(p.method_selector, 'hex') AS selector_hex,
        p.settlement_kind,
        count(*) AS txn_count,
        sum(p.amount_usdc) AS volume_usdc,
        min(p.methodology_version) AS mv
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE w.days = 0 OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
    GROUP BY w.window_name,
        GROUPING SETS ((CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END), ()),
        encode(p.method_selector, 'hex'), p.settlement_kind
)
INSERT INTO metrics_mechanics_selector_v2
    (window_name, membership, rank, methodology_version, selector_hex, settlement_kind, txn_count, volume_usdc)
SELECT window_name, membership, rk, mv, selector_hex, settlement_kind, txn_count, volume_usdc
FROM (
    SELECT *, row_number() OVER (
        PARTITION BY window_name, membership
        ORDER BY txn_count DESC, selector_hex, settlement_kind) AS rk
    FROM agg
) ranked
WHERE rk <= 15;`

// RebuildMechanics recomputes the mechanics rollup tables from payment_x402_v1.
// Called by Rebuild after RebuildReliability inside the same REPEATABLE READ tx.
func RebuildMechanics(ctx context.Context, tx pgx.Tx) error {
	for _, s := range []struct{ name, sql string }{
		{"window", mechanicsWindowSQL},
		{"batch", mechanicsBatchSQL},
		{"block", mechanicsBlockSQL},
		{"selector", mechanicsSelectorSQL},
	} {
		if _, err := tx.Exec(ctx, s.sql); err != nil {
			return fmt.Errorf("mechanics %s rollup: %w", s.name, err)
		}
	}
	return nil
}
