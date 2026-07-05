-- +goose Up
-- +goose StatementBegin
-- Per-day payee counts for the top-12 all-window price points (item 6.7).
-- Populated by the rollup via priceBreadthDailySQL; read by BuildPriceBreadth
-- in emit to feed the price-point sparklines on the economy page.
CREATE TABLE IF NOT EXISTS metrics_price_point_daily_v1 (
    day                 DATE          NOT NULL,
    amount_usdc         NUMERIC(38,6) NOT NULL,
    payee_count         BIGINT        NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    PRIMARY KEY (day, amount_usdc)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_price_point_daily_v1;
-- +goose StatementEnd
