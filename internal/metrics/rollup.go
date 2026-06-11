package metrics

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rebuildCubeSQL recomputes the cube in one pass. day is the UTC calendar day.
// attribution comes from the classification view; amount_band from the
// migration's IMMUTABLE function. TRUNCATE + INSERT makes the operation
// idempotent and safe to re-run after any backfill.
const rebuildCubeSQL = `
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

// missingPriceMonthsSQL lists months present in payments but absent from the
// session's eth_price_monthly temp table. NULL result = full coverage.
const missingPriceMonthsSQL = `
SELECT string_agg(m, ', ' ORDER BY m) FROM (
    SELECT DISTINCT to_char(block_timestamp AT TIME ZONE 'UTC', 'YYYY-MM') AS m FROM payments
    EXCEPT
    SELECT month FROM eth_price_monthly
) missing`

// rebuildStatements are run in order inside one transaction. Each statement is
// its own TRUNCATE + INSERT, so a failure anywhere rolls the whole generation
// back and the previous tables stay live.
var rebuildStatements = []string{
	rebuildCubeSQL,
}

// Rebuild fully recomputes every metrics table from payment_classified_v1 in a
// single transaction. prices is the curated monthly ETH/USD reference (already
// validated by LoadETHPrices); it is staged into a temp table so the gas SQL
// can join it, and coverage is checked against payments BEFORE any TRUNCATE.
func Rebuild(ctx context.Context, pool *pgxpool.Pool, prices ETHPrices) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rebuild tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Percentile sorts over the windowed CROSS JOIN spill to temp; cap the
	// spill so a runaway plan cannot fill the host disk.
	if _, err := tx.Exec(ctx, `SET LOCAL temp_file_limit = '30GB'`); err != nil {
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

	for i, stmt := range rebuildStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("rebuild statement %d: %w", i, err)
		}
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
