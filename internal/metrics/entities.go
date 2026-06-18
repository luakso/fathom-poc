package metrics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// entityRoles maps a role to its (entity column, counterparty column) in
// payment_x402_v1. role is a fixed internal constant, never user input, so it is
// safe to interpolate into SQL.
var entityRoles = []struct{ role, entityCol, counterpartyCol string }{
	{"payee", "payee", "payer"},
	{"payer", "payer", "payee"},
}

// RebuildEntities recomputes the three entity tables for every (window, role)
// from payment_x402_v1. Called by Rebuild inside its REPEATABLE READ transaction
// AFTER the cube statements, so all artifacts share one snapshot. Per role it
// materializes a per-(window,entity) aggregate into a temp table (one scan), then
// derives the leaderboard union, the bucket histogram, and the concentration
// summary from it. Exact distinct counts (no HLL); the tx's temp_file_limit guard
// covers the entity-grain spill.
func RebuildEntities(ctx context.Context, tx pgx.Tx) error {
	for _, t := range []string{"entity_rank_v1", "entity_buckets_v1", "entity_concentration_v1"} {
		if _, err := tx.Exec(ctx, "TRUNCATE "+t); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	for _, r := range entityRoles {
		if err := rebuildEntityRole(ctx, tx, r.role, r.entityCol, r.counterpartyCol); err != nil {
			return fmt.Errorf("rebuild entities %s: %w", r.role, err)
		}
	}
	return nil
}

// rebuildEntityRole builds a fresh entity_agg temp table for one role and inserts
// the three derived projections. entity_agg is dropped at the end so the next role
// recreates it; the surrounding tx already sets temp_file_limit.
func rebuildEntityRole(ctx context.Context, tx pgx.Tx, role, entityCol, counterpartyCol string) error {
	aggSQL := fmt.Sprintf(`
CREATE TEMP TABLE entity_agg AS
SELECT
    w.window_name,
    p.%[1]s                                   AS address,
    sum(p.amount_usdc)                        AS volume_usdc,
    count(*)                                  AS txn_count,
    count(DISTINCT p.%[2]s)                   AS distinct_counterparties,
    count(DISTINCT p.amount_usdc)             AS distinct_amounts,
    COALESCE(sum(p.amount_usdc) FILTER (WHERE p.facilitator_known), 0) AS known_volume_usdc,
    min(p.block_timestamp)                    AS first_seen,
    max(p.block_timestamp)                    AS last_seen,
    min(p.methodology_version)                AS methodology_version
FROM payment_x402_v1 p
CROSS JOIN %[3]s
CROSS JOIN (SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1) anchor
WHERE w.days = 0
   OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= anchor.d - (w.days - 1)
GROUP BY w.window_name, p.%[1]s`, entityCol, counterpartyCol, windowsValues)

	if _, err := tx.Exec(ctx, aggSQL); err != nil {
		return fmt.Errorf("build entity_agg: %w", err)
	}

	// Leaderboard: union of top-150-by-volume and top-150-by-txns per window.
	rankSQL := fmt.Sprintf(`
INSERT INTO entity_rank_v1
    (window_name, role, address, volume_usdc, txn_count, distinct_counterparties,
     distinct_amounts, known_volume_usdc, first_seen, last_seen, methodology_version)
SELECT window_name, '%[1]s', address, volume_usdc, txn_count, distinct_counterparties,
       distinct_amounts, known_volume_usdc, first_seen, last_seen, methodology_version
FROM (
    SELECT *,
        row_number() OVER (PARTITION BY window_name ORDER BY volume_usdc DESC, address) AS rv,
        row_number() OVER (PARTITION BY window_name ORDER BY txn_count  DESC, address) AS rt
    FROM entity_agg
) ranked
WHERE rv <= 150 OR rt <= 150`, role)
	if _, err := tx.Exec(ctx, rankSQL); err != nil {
		return fmt.Errorf("insert entity_rank: %w", err)
	}

	bucketSQL := fmt.Sprintf(`
INSERT INTO entity_buckets_v1
    (window_name, role, bucket, entity_count, txn_sum, volume_sum, methodology_version)
SELECT window_name, '%[1]s', entity_txn_bucket(txn_count),
       count(*), sum(txn_count), sum(volume_usdc), min(methodology_version)
FROM entity_agg
GROUP BY window_name, entity_txn_bucket(txn_count)`, role)
	if _, err := tx.Exec(ctx, bucketSQL); err != nil {
		return fmt.Errorf("insert entity_buckets: %w", err)
	}

	concSQL := fmt.Sprintf(`
INSERT INTO entity_concentration_v1
    (window_name, role, total_entities, total_volume, total_txns,
     top10_volume, top10_txns, top100_volume, methodology_version)
SELECT window_name, '%[1]s',
       count(*),
       sum(volume_usdc),
       sum(txn_count),
       COALESCE(sum(volume_usdc) FILTER (WHERE rv <= 10), 0),
       COALESCE(sum(txn_count)   FILTER (WHERE rt <= 10), 0),
       COALESCE(sum(volume_usdc) FILTER (WHERE rv <= 100), 0),
       min(methodology_version)
FROM (
    SELECT window_name, volume_usdc, txn_count, methodology_version,
        row_number() OVER (PARTITION BY window_name ORDER BY volume_usdc DESC, address) AS rv,
        row_number() OVER (PARTITION BY window_name ORDER BY txn_count  DESC, address) AS rt
    FROM entity_agg
) ranked
GROUP BY window_name`, role)
	if _, err := tx.Exec(ctx, concSQL); err != nil {
		return fmt.Errorf("insert entity_concentration: %w", err)
	}

	if _, err := tx.Exec(ctx, "DROP TABLE entity_agg"); err != nil {
		return fmt.Errorf("drop entity_agg: %w", err)
	}
	return nil
}
