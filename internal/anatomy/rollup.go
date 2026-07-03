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

// rollupStatements run in order inside one transaction. Later tasks append
// price points, leaderboard, and the meta stamp.
var rollupStatements = []struct{ name, sql string }{
	{"entity_edge_v1", rebuildEntityEdgeSQL},
	{"facilitator_edge_v1", rebuildFacilitatorEdgeSQL},
	{"entity_day_v1", rebuildEntityDaySQL},
}

// rollupTables is everything Rollup rebuilds; ANALYZE runs on each after commit.
var rollupTables = []string{
	"entity_edge_v1", "facilitator_edge_v1", "entity_day_v1",
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
		return err
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
