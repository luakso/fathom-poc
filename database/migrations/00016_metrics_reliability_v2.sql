-- +goose Up
-- +goose StatementBegin
-- v2 reliability rollup (R1–R4, R6). Window-anchored summary keyed on MEMBERSHIP
-- (known/unknown/all), sourced from payment_x402_v1 + authorization_cancellation_v1.
-- Latency percentiles are NOT mergeable from a daily grain, so they are computed
-- per-window here (like metrics_window_stats_v2). R2 latency is over the WINDOWED
-- subset only (rows carrying both valid_after and valid_before); windowed_count
-- exposes that denominator so no rate is misread as "% of all settlements".
-- R5 (tx_nonce gaps) is deliberately excluded: tx_nonce is the facilitator EOA
-- nonce, which increments on every (incl. non-x402) tx, so gaps cannot mean
-- dropped settlements on an x402-only table. See the design spec.
CREATE TABLE IF NOT EXISTS metrics_reliability_window_v2 (
    window_name         TEXT     NOT NULL,           -- '7d' | '30d' | 'all'
    membership          TEXT     NOT NULL,           -- 'known' | 'unknown' | 'all'
    methodology_version SMALLINT NOT NULL,
    settlement_count    BIGINT   NOT NULL,           -- all settlements in (window, membership)
    windowed_count      BIGINT   NOT NULL,           -- rows with valid_after AND valid_before
    cancellation_count  BIGINT   NOT NULL DEFAULT 0, -- R1 numerator
    latency_p50_s       DOUBLE PRECISION,            -- R2; NULL when no windowed rows
    latency_p90_s       DOUBLE PRECISION,
    latency_p99_s       DOUBLE PRECISION,
    lat_bucket_sub1s    BIGINT   NOT NULL DEFAULT 0, -- R2 histogram (non-negative latency only)
    lat_bucket_1_10s    BIGINT   NOT NULL DEFAULT 0,
    lat_bucket_10_60s   BIGINT   NOT NULL DEFAULT 0,
    lat_bucket_1_10m    BIGINT   NOT NULL DEFAULT 0,
    lat_bucket_gt10m    BIGINT   NOT NULL DEFAULT 0,
    expired_count       BIGINT   NOT NULL DEFAULT 0, -- R3: block_timestamp > valid_before
    not_yet_valid_count BIGINT   NOT NULL DEFAULT 0, -- R4: block_timestamp < valid_after
    PRIMARY KEY (window_name, membership)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Day x membership trend for R1/R3/R4 (no 'all' row — page sums as needed, like
-- metrics_velocity_daily_v2). cancellation_count joined by block_time::date.
CREATE TABLE IF NOT EXISTS metrics_reliability_daily_v2 (
    day                 DATE     NOT NULL,
    chain               TEXT     NOT NULL,
    membership          TEXT     NOT NULL,           -- 'known' | 'unknown'
    methodology_version SMALLINT NOT NULL,
    settlement_count    BIGINT   NOT NULL,
    windowed_count      BIGINT   NOT NULL,
    expired_count       BIGINT   NOT NULL DEFAULT 0,
    not_yet_valid_count BIGINT   NOT NULL DEFAULT 0,
    cancellation_count  BIGINT   NOT NULL DEFAULT 0,
    PRIMARY KEY (day, chain, membership)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_reliability_daily_v2;
DROP TABLE IF EXISTS metrics_reliability_window_v2;
-- +goose StatementEnd
