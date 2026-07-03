package anatomy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// anatomyMethodologyVersion stamps every rollup artifact. Bump only with a
// methodology change (and a matching view/table version bump).
// Used by the meta stamp in Task 6; defined here so it is visible to all
// rollup helpers in this package.
const anatomyMethodologyVersion = 1 //nolint:unused

// rollupTempFileLimit guards the distinct-count and window sorts (publisher
// convention; superuser-only GUC - the local/prod user is the bootstrap
// superuser).
const rollupTempFileLimit = "30GB"

// rebuildEntityEdgeSQL: one scan; TRUNCATE+INSERT keeps it idempotent.
const rebuildEntityEdgeSQL = `
TRUNCATE entity_edge_v1;
INSERT INTO entity_edge_v1
    (chain, payer, payee, facilitator_known, txn_count, volume_usdc,
     first_seen, last_seen, methodology_version)
SELECT chain, payer, payee, facilitator_known,
       count(*), sum(amount_usdc), min(block_timestamp), max(block_timestamp),
       min(methodology_version)
FROM payment_x402_v1
GROUP BY chain, payer, payee, facilitator_known`

// rebuildFacilitatorEdgeSQL: LATERAL VALUES unpivots each payment into a
// payer-side and a payee-side counterparty row (facilitators are not part of
// entity_edge_v1, so facilitator nodes stay expandable via this table).
const rebuildFacilitatorEdgeSQL = `
TRUNCATE facilitator_edge_v1;
INSERT INTO facilitator_edge_v1
    (chain, facilitator, counterparty_role, counterparty, facilitator_known,
     txn_count, volume_usdc, first_seen, last_seen, methodology_version)
SELECT p.chain, p.facilitator, r.role, r.address, p.facilitator_known,
       count(*), sum(p.amount_usdc), min(p.block_timestamp), max(p.block_timestamp),
       min(p.methodology_version)
FROM payment_x402_v1 p
CROSS JOIN LATERAL (VALUES (p.payer, 'payer'), (p.payee, 'payee')) AS r(address, role)
GROUP BY p.chain, p.facilitator, r.role, r.address, p.facilitator_known`

// rebuildEntityDaySQL: one scan; each payment lands once per role slice.
const rebuildEntityDaySQL = `
TRUNCATE entity_day_v1;
INSERT INTO entity_day_v1
    (chain, address, role, day, facilitator_known, txn_count, volume_usdc,
     methodology_version)
SELECT p.chain, r.address, r.role,
       (p.block_timestamp AT TIME ZONE 'UTC')::date, p.facilitator_known,
       count(*), sum(p.amount_usdc), min(p.methodology_version)
FROM payment_x402_v1 p
CROSS JOIN LATERAL (VALUES
    (p.payer, 'payer'), (p.payee, 'payee'), (p.facilitator, 'facilitator')
) AS r(address, role)
GROUP BY p.chain, r.address, r.role,
         (p.block_timestamp AT TIME ZONE 'UTC')::date, p.facilitator_known`

// rebuildEntityPricePointSQL: per-(address, role, lens) amount histogram,
// capped at the 64 most frequent amounts. total_distinct_amounts is
// denormalized onto every stored row so "single price point" claims stay
// honest even when the tail is truncated.
const rebuildEntityPricePointSQL = `
TRUNCATE entity_price_point_v1;
INSERT INTO entity_price_point_v1
    (chain, address, role, facilitator_known, amount_usdc, txn_count,
     amount_rank, total_distinct_amounts, methodology_version)
SELECT chain, address, role, facilitator_known, amount_usdc, txn_count,
       amount_rank, total_distinct_amounts, methodology_version
FROM (
    SELECT chain, address, role, facilitator_known, amount_usdc, txn_count,
           methodology_version,
           row_number() OVER (
               PARTITION BY chain, address, role, facilitator_known
               ORDER BY txn_count DESC, amount_usdc
           ) AS amount_rank,
           count(*) OVER (
               PARTITION BY chain, address, role, facilitator_known
           ) AS total_distinct_amounts
    FROM (
        SELECT p.chain, r.address, r.role, p.facilitator_known, p.amount_usdc,
               count(*) AS txn_count, min(p.methodology_version) AS methodology_version
        FROM payment_x402_v1 p
        CROSS JOIN LATERAL (VALUES
            (p.payer, 'payer'), (p.payee, 'payee'), (p.facilitator, 'facilitator')
        ) AS r(address, role)
        GROUP BY p.chain, r.address, r.role, p.facilitator_known, p.amount_usdc
    ) amounts
) ranked
WHERE amount_rank <= 64`

// rebuildEntityLeaderboardSQL: per-(window, role, lens) top-500 union across
// the three sort metrics. Windows anchor to max(data day), not wall clock
// (dashboard convention). lens is a first-class dimension because rankings
// are not mergeable across facilitator_known. The base unpivot is
// windows x roles x lens-expansion of the view; temp_file_limit guards the
// sort spill.
const rebuildEntityLeaderboardSQL = `
TRUNCATE entity_leaderboard_v1;
WITH anchor AS (
    SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1
), agg AS (
    SELECT w.window_name, r.role, l.lens, r.address,
           count(*)                    AS txn_count,
           sum(p.amount_usdc)          AS volume_usdc,
           count(DISTINCT r.counterparty) AS distinct_counterparties,
           min(p.block_timestamp)      AS first_seen,
           max(p.block_timestamp)      AS last_seen,
           min(p.methodology_version)  AS methodology_version
    FROM payment_x402_v1 p
    CROSS JOIN anchor a
    CROSS JOIN (VALUES ('7d', 7), ('30d', 30), ('all', 0)) AS w(window_name, days)
    CROSS JOIN LATERAL (VALUES
        (p.payer, 'payer', p.payee), (p.payee, 'payee', p.payer)
    ) AS r(address, role, counterparty)
    CROSS JOIN LATERAL (VALUES ('all'), ('known')) AS l(lens)
    WHERE (w.days = 0
       OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= a.d - (w.days - 1))
      AND (l.lens = 'all' OR p.facilitator_known)
    GROUP BY w.window_name, r.role, l.lens, r.address
), ranked AS (
    SELECT *,
        row_number() OVER (PARTITION BY window_name, role, lens
                           ORDER BY volume_usdc DESC, address) AS rv,
        row_number() OVER (PARTITION BY window_name, role, lens
                           ORDER BY txn_count DESC, address) AS rt,
        row_number() OVER (PARTITION BY window_name, role, lens
                           ORDER BY distinct_counterparties DESC, address) AS rc
    FROM agg
)
INSERT INTO entity_leaderboard_v1
    (window_name, role, lens, address, txn_count, volume_usdc,
     distinct_counterparties, first_seen, last_seen, methodology_version)
SELECT window_name, role, lens, address, txn_count, volume_usdc,
       distinct_counterparties, first_seen, last_seen, methodology_version
FROM ranked
WHERE rv <= 500 OR rt <= 500 OR rc <= 500`

// rollupStatements run in order inside one transaction. Later tasks append
// price points, leaderboard, and the meta stamp.
var rollupStatements = []struct{ name, sql string }{
	{"entity_edge_v1", rebuildEntityEdgeSQL},
	{"facilitator_edge_v1", rebuildFacilitatorEdgeSQL},
	{"entity_day_v1", rebuildEntityDaySQL},
	{"entity_price_point_v1", rebuildEntityPricePointSQL},
	{"entity_leaderboard_v1", rebuildEntityLeaderboardSQL},
}

// rollupTables is everything Rollup rebuilds; ANALYZE runs on each after commit.
var rollupTables = []string{
	"entity_edge_v1", "facilitator_edge_v1", "entity_day_v1",
	"entity_price_point_v1", "entity_leaderboard_v1",
}

// Rollup rebuilds all anatomy entity tables from payment_x402_v1 in one
// REPEATABLE READ transaction (all artifacts share one snapshot), replaces the
// manual identity signals from the curated label file, and ANALYZEs the
// rebuilt tables. Idempotent; safe to re-run after any backfill.
func Rollup(ctx context.Context, pool *pgxpool.Pool, labels []ManualLabel) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return fmt.Errorf("begin rollup tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, fmt.Sprintf(`SET LOCAL temp_file_limit = '%s'`, rollupTempFileLimit)); err != nil {
		return fmt.Errorf("set temp_file_limit: %w", err)
	}
	for _, st := range rollupStatements {
		if _, err := tx.Exec(ctx, st.sql); err != nil {
			return fmt.Errorf("rebuild %s: %w", st.name, err)
		}
	}
	if err := replaceManualSignals(ctx, tx, labels); err != nil {
		return fmt.Errorf("rollup manual signals: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollup: %w", err)
	}
	for _, tbl := range rollupTables {
		if _, err := pool.Exec(ctx, "ANALYZE "+tbl); err != nil {
			return fmt.Errorf("analyze %s: %w", tbl, err)
		}
	}
	return nil
}

// replaceManualSignals swaps the source='manual' rows for the curated set.
// A nil/empty slice clears manual labels (the file is the source of truth).
func replaceManualSignals(ctx context.Context, tx pgx.Tx, labels []ManualLabel) error {
	if _, err := tx.Exec(ctx, `DELETE FROM entity_signal WHERE source = 'manual'`); err != nil {
		return fmt.Errorf("clear manual signals: %w", err)
	}
	for _, l := range labels {
		var url *string
		if l.URL != "" {
			url = &l.URL
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO entity_signal (chain, address, source, kind, value, url, meta)
			VALUES ($1, $2, 'manual', 'label', $3, $4, jsonb_build_object('note', $5::text))`,
			l.Chain, l.Address, l.Label, url, l.Note); err != nil {
			return fmt.Errorf("insert manual signal %s: %w", l.Address, err)
		}
	}
	return nil
}
