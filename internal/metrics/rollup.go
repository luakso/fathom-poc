package metrics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// rebuildDailySQL recomputes the whole cube in one pass. day is the UTC calendar
// day. attribution comes from the classification view; amount_band from the
// migration's IMMUTABLE function. TRUNCATE + INSERT makes the operation
// idempotent and safe to re-run after any backfill (including backfills that
// fill old gaps, which a max-day incremental could miss).
const rebuildDailySQL = `
TRUNCATE metrics_daily_v1;
INSERT INTO metrics_daily_v1
    (day, chain, facilitator, attribution, amount_band, methodology_version, txn_count, volume_usdc, max_amount_usdc)
SELECT
    (block_timestamp AT TIME ZONE 'UTC')::date AS day,
    chain,
    facilitator,
    attribution,
    amount_band(amount_usdc) AS amount_band,
    methodology_version,
    count(*)                  AS txn_count,
    sum(amount_usdc)          AS volume_usdc,
    max(amount_usdc)          AS max_amount_usdc
FROM payment_classified_v1
GROUP BY 1, 2, 3, 4, 5, 6`

// RebuildDaily fully recomputes metrics_daily_v1 from payment_classified_v1.
// Runs TRUNCATE + INSERT in one transaction so a failure leaves the previous
// cube intact.
func RebuildDaily(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rebuild tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, rebuildDailySQL); err != nil {
		return fmt.Errorf("rebuild metrics_daily_v1: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rebuild: %w", err)
	}
	return nil
}
