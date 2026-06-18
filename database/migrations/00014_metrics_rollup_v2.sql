-- +goose Up
-- +goose StatementBegin
-- v2 cube. Same grain as the v1 set but keyed on MEMBERSHIP (known/unknown
-- facilitator) instead of attribution, sourced from payment_x402_v1. v1 tables
-- are left frozen. amount_band(numeric) already exists (migration 00009).
CREATE TABLE IF NOT EXISTS metrics_daily_v2 (
    day          DATE          NOT NULL,
    chain        TEXT          NOT NULL,
    facilitator  TEXT          NOT NULL,
    membership   TEXT          NOT NULL,  -- 'known' | 'unknown'
    amount_band  TEXT          NOT NULL,
    methodology_version SMALLINT NOT NULL,
    txn_count    BIGINT        NOT NULL,
    volume_usdc  NUMERIC(38,6) NOT NULL,
    max_amount_usdc NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (day, chain, facilitator, membership, amount_band)
);
CREATE INDEX IF NOT EXISTS idx_metrics_daily_v2_facilitator ON metrics_daily_v2(facilitator);
-- +goose StatementEnd

-- +goose StatementBegin
-- Per-(window, membership) medians (NOT mergeable from the cube). membership
-- carries the synthetic 'all' total row (medians can't be summed from
-- known+unknown), exactly as the v1 table used 'all'.
CREATE TABLE IF NOT EXISTS metrics_window_stats_v2 (
    window_name         TEXT          NOT NULL,  -- '7d' | '30d' | 'all'
    membership          TEXT          NOT NULL,  -- 'known' | 'unknown' | 'all'
    methodology_version SMALLINT      NOT NULL,
    txn_count           BIGINT        NOT NULL,
    median_amount_usdc  NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (window_name, membership)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Top exact amounts per window over the KNOWN-facilitator set (E8).
CREATE TABLE IF NOT EXISTS metrics_price_points_v2 (
    window_name         TEXT          NOT NULL,
    rank                INTEGER       NOT NULL,
    amount_usdc         NUMERIC(38,6) NOT NULL,
    txn_count           BIGINT        NOT NULL,
    volume_usdc         NUMERIC(38,6) NOT NULL,
    payee_count         BIGINT        NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    PRIMARY KEY (window_name, rank)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Settlement cost at (day, membership, band) grain. TRUE L2 cost is split into
-- its components: l2_gas_cost_wei (execution) + l1_fee_wei (data/calldata).
-- cost_usd and breakeven use the TOTAL (l1+l2). Both component sums feed the
-- L1-vs-L2 share (E13) with no extra scan. All apportioned tx-wise (see rollup).
CREATE TABLE IF NOT EXISTS metrics_gas_daily_v2 (
    day                 DATE          NOT NULL,
    membership          TEXT          NOT NULL,
    amount_band         TEXT          NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    txn_count           BIGINT        NOT NULL,
    l2_gas_cost_wei     NUMERIC       NOT NULL,  -- apportioned execution gas
    l1_fee_wei          NUMERIC       NOT NULL,  -- apportioned L1 data fee
    cost_usd            NUMERIC(38,8) NOT NULL,  -- total (l1+l2) in USD
    breakeven_txn_count BIGINT        NOT NULL,  -- payments whose total cost > amount
    volume_usdc         NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (day, membership, amount_band)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Burstiness at (day, membership) grain.
CREATE TABLE IF NOT EXISTS metrics_velocity_daily_v2 (
    day                 DATE     NOT NULL,
    membership          TEXT     NOT NULL,
    methodology_version SMALLINT NOT NULL,
    txn_count           BIGINT   NOT NULL,
    max_per_min         INTEGER  NOT NULL,
    p99_per_min         INTEGER  NOT NULL,
    PRIMARY KEY (day, membership)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_velocity_daily_v2;
DROP TABLE IF EXISTS metrics_gas_daily_v2;
DROP TABLE IF EXISTS metrics_price_points_v2;
DROP TABLE IF EXISTS metrics_window_stats_v2;
DROP TABLE IF EXISTS metrics_daily_v2;
-- +goose StatementEnd
