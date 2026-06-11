-- +goose Up
-- +goose StatementBegin
-- metrics_window_stats_v1: per-(window, attribution) stats that are NOT
-- mergeable from any cube (medians). attribution includes the synthetic 'all'.
-- Windows are anchored at rollup time to max(day) of the data — the same
-- anchor emit uses (data_through_day), so the two always agree.
CREATE TABLE IF NOT EXISTS metrics_window_stats_v1 (
    window_name         TEXT          NOT NULL,  -- '7d' | '30d' | 'all'
    attribution         TEXT          NOT NULL,  -- 'agentic'|'contested'|'contamination'|'all'
    methodology_version SMALLINT      NOT NULL,
    txn_count           BIGINT        NOT NULL,
    median_amount_usdc  NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (window_name, attribution)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- metrics_price_points_v1: top exact amounts in the AGENTIC set per window
-- (E8 "menu vs market"). Only the top 50 by txn count are publishable out of
-- ~340k distinct agentic amounts.
CREATE TABLE IF NOT EXISTS metrics_price_points_v1 (
    window_name         TEXT          NOT NULL,
    rank                INTEGER       NOT NULL,  -- 1-based, by txn_count desc
    amount_usdc         NUMERIC(38,6) NOT NULL,
    txn_count           BIGINT        NOT NULL,
    volume_usdc         NUMERIC(38,6) NOT NULL,
    payee_count         BIGINT        NOT NULL,  -- distinct payees at this amount
    methodology_version SMALLINT      NOT NULL,
    PRIMARY KEY (window_name, rank)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- metrics_gas_daily_v1: settlement gas at (day, attribution, band) grain.
-- gas_cost_wei is tx-level in payments (duplicated per row of a batch), so the
-- rollup dedupes per (chain, tx_hash) and apportions tx_gas/n_payments equally
-- across the tx's payments; the apportioned sum conserves the per-tx sum to
-- ~16 significant figures (sub-wei drift). gas_cost_usd uses the curated
-- monthly ETH/USD reference. breakeven
-- counts payments whose apportioned gas in USD exceeds the amount moved.
CREATE TABLE IF NOT EXISTS metrics_gas_daily_v1 (
    day                 DATE          NOT NULL,
    attribution         TEXT          NOT NULL,
    amount_band         TEXT          NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    txn_count           BIGINT        NOT NULL,
    gas_cost_wei        NUMERIC       NOT NULL,  -- apportioned; fractional wei preserved
    gas_cost_usd        NUMERIC(38,8) NOT NULL,
    breakeven_txn_count BIGINT        NOT NULL,
    volume_usdc         NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (day, attribution, amount_band)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- metrics_velocity_daily_v1: burstiness at (day, attribution) grain, reduced
-- from per-minute counts. p99 is over the day's ACTIVE minutes (≥1 payment) —
-- idle minutes would zero the percentile on quiet days.
CREATE TABLE IF NOT EXISTS metrics_velocity_daily_v1 (
    day                 DATE     NOT NULL,
    attribution         TEXT     NOT NULL,
    methodology_version SMALLINT NOT NULL,
    txn_count           BIGINT   NOT NULL,
    max_per_min         INTEGER  NOT NULL,
    p99_per_min         INTEGER  NOT NULL,
    PRIMARY KEY (day, attribution)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_velocity_daily_v1;
DROP TABLE IF EXISTS metrics_gas_daily_v1;
DROP TABLE IF EXISTS metrics_price_points_v1;
DROP TABLE IF EXISTS metrics_window_stats_v1;
-- +goose StatementEnd
