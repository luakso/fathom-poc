-- +goose Up
-- +goose StatementBegin
-- Add p10/p90/p99 amount columns to metrics_window_stats_v2 (item 6.3).
-- Nullable so existing rows (populated before this migration) stay valid without
-- a data migration; the rollup TRUNCATE + INSERT repopulates the table on the
-- next publisher rollup run.
ALTER TABLE metrics_window_stats_v2
    ADD COLUMN IF NOT EXISTS p10_amount_usdc NUMERIC(38,6),
    ADD COLUMN IF NOT EXISTS p90_amount_usdc NUMERIC(38,6),
    ADD COLUMN IF NOT EXISTS p99_amount_usdc NUMERIC(38,6);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE metrics_window_stats_v2
    DROP COLUMN IF EXISTS p10_amount_usdc,
    DROP COLUMN IF EXISTS p90_amount_usdc,
    DROP COLUMN IF EXISTS p99_amount_usdc;
-- +goose StatementEnd
