package metrics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// reliabilityWindowSQL computes the per-(window, membership) reliability summary.
// One scan of payment_x402_v1 over a windows CROSS JOIN; latency, expired, and
// not-yet-valid are conditional aggregates over the WINDOWED subset (both auth
// bounds present). 'all' membership comes from GROUPING SETS. Cancellations are
// folded in by a following UPDATE (separate source table). latency uses
// percentile_cont over EPOCH seconds, FILTERed to non-negative latency (a
// before-valid_after settlement is R4, not latency). See the design spec.
const reliabilityWindowSQL = `
TRUNCATE metrics_reliability_window_v2;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
),
base AS (
    SELECT
        w.window_name,
        CASE WHEN p.facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
        p.methodology_version,
        (p.valid_after IS NOT NULL AND p.valid_before IS NOT NULL) AS windowed,
        -- expired/not_yet_valid are gated on the WINDOWED subset (both bounds) so the
        -- numerator stays a subset of windowed_count — the rate denominator — and can
        -- never exceed 1. (EIP-3009 co-presents both bounds, so this is belt-and-braces.)
        (p.valid_after IS NOT NULL AND p.valid_before IS NOT NULL AND p.block_timestamp > p.valid_before) AS expired,
        (p.valid_after IS NOT NULL AND p.valid_before IS NOT NULL AND p.block_timestamp < p.valid_after)  AS not_yet_valid,
        CASE WHEN p.valid_after IS NOT NULL AND p.valid_before IS NOT NULL
             THEN EXTRACT(EPOCH FROM (p.block_timestamp - p.valid_after)) END AS latency_s
    FROM payment_x402_v1 p
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN anchor a
    WHERE w.days = 0
       OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
)
INSERT INTO metrics_reliability_window_v2
    (window_name, membership, methodology_version, settlement_count, windowed_count,
     cancellation_count, latency_p50_s, latency_p90_s, latency_p99_s,
     lat_bucket_sub1s, lat_bucket_1_10s, lat_bucket_10_60s, lat_bucket_1_10m, lat_bucket_gt10m,
     expired_count, not_yet_valid_count)
SELECT
    window_name,
    COALESCE(membership, 'all'),
    min(methodology_version),
    count(*),
    count(*) FILTER (WHERE windowed),
    0,
    percentile_cont(0.5)  WITHIN GROUP (ORDER BY latency_s) FILTER (WHERE latency_s >= 0),
    percentile_cont(0.9)  WITHIN GROUP (ORDER BY latency_s) FILTER (WHERE latency_s >= 0),
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_s) FILTER (WHERE latency_s >= 0),
    count(*) FILTER (WHERE latency_s >= 0    AND latency_s < 1),
    count(*) FILTER (WHERE latency_s >= 1    AND latency_s < 10),
    count(*) FILTER (WHERE latency_s >= 10   AND latency_s < 60),
    count(*) FILTER (WHERE latency_s >= 60   AND latency_s < 600),
    count(*) FILTER (WHERE latency_s >= 600),
    count(*) FILTER (WHERE expired),
    count(*) FILTER (WHERE not_yet_valid)
FROM base
GROUP BY window_name, GROUPING SETS ((membership), ());

UPDATE metrics_reliability_window_v2 t
SET cancellation_count = c.cnt
FROM (
    SELECT
        w.window_name,
        COALESCE(CASE WHEN x.facilitator_known THEN 'known' ELSE 'unknown' END, 'all') AS membership,
        count(*) AS cnt
    FROM authorization_cancellation_v1 x
    CROSS JOIN ` + windowsValues + `
    CROSS JOIN (SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1) a
    WHERE w.days = 0
       OR (x.block_time AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1)
    GROUP BY w.window_name,
        GROUPING SETS ((CASE WHEN x.facilitator_known THEN 'known' ELSE 'unknown' END), ())
) c
WHERE t.window_name = c.window_name AND t.membership = c.membership;`

// reliabilityDailySQL computes the day x membership trend (R1/R3/R4). No 'all'
// row (the page sums). cancellation_count joins by block_time::date; days with
// payments but no cancellations stay 0. A cancellation whose (day, membership)
// has no settlement row is dropped (tiny-fixture edge; noted in spec).
const reliabilityDailySQL = `
TRUNCATE metrics_reliability_daily_v2;
INSERT INTO metrics_reliability_daily_v2
    (day, chain, membership, methodology_version, settlement_count, windowed_count,
     expired_count, not_yet_valid_count, cancellation_count)
SELECT
    (block_timestamp AT TIME ZONE 'UTC')::date,
    chain,
    CASE WHEN facilitator_known THEN 'known' ELSE 'unknown' END,
    min(methodology_version),
    count(*),
    count(*) FILTER (WHERE valid_after IS NOT NULL AND valid_before IS NOT NULL),
    count(*) FILTER (WHERE valid_after IS NOT NULL AND valid_before IS NOT NULL AND block_timestamp > valid_before),
    count(*) FILTER (WHERE valid_after IS NOT NULL AND valid_before IS NOT NULL AND block_timestamp < valid_after),
    0
FROM payment_x402_v1
GROUP BY 1, 2, 3;

UPDATE metrics_reliability_daily_v2 t
SET cancellation_count = c.cnt
FROM (
    SELECT
        (block_time AT TIME ZONE 'UTC')::date AS day,
        chain,
        CASE WHEN facilitator_known THEN 'known' ELSE 'unknown' END AS membership,
        count(*) AS cnt
    FROM authorization_cancellation_v1
    GROUP BY 1, 2, 3
) c
WHERE t.day = c.day AND t.chain = c.chain AND t.membership = c.membership;`

// RebuildReliability recomputes the reliability rollup tables from
// payment_x402_v1 + authorization_cancellation_v1. Called by Rebuild inside its
// REPEATABLE READ transaction after RebuildEntities, so it shares the snapshot
// and temp_file_limit. Each statement string is its own TRUNCATE + INSERT + UPDATE.
func RebuildReliability(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, reliabilityWindowSQL); err != nil {
		return fmt.Errorf("reliability window rollup: %w", err)
	}
	if _, err := tx.Exec(ctx, reliabilityDailySQL); err != nil {
		return fmt.Errorf("reliability daily rollup: %w", err)
	}
	return nil
}
