-- +goose Up
-- +goose StatementBegin
-- v2 mechanics rollup. Per-payment-grain stats keyed on window x membership.
-- M4 fee intent, M7 auth-window width, M9 gas over-provisioning, M11 tx_value
-- canary, M12 data hygiene. Cost (M1/M10/E11/E13) is NOT here — it is reused from
-- metrics_gas_daily_v2 at emit time. M8 (calldata decode) and M6 (nonce forensics)
-- are deferred. See the design spec.
CREATE TABLE IF NOT EXISTS metrics_mechanics_window_v2 (
    window_name             TEXT     NOT NULL,            -- '7d' | '30d' | 'all'
    membership              TEXT     NOT NULL,            -- 'known' | 'unknown' | 'all'
    methodology_version     SMALLINT NOT NULL,
    settlement_count        BIGINT   NOT NULL,
    tx_type_0_count         BIGINT   NOT NULL DEFAULT 0,  -- M4
    tx_type_1_count         BIGINT   NOT NULL DEFAULT 0,
    tx_type_2_count         BIGINT   NOT NULL DEFAULT 0,
    max_fee_p50             NUMERIC,                       -- M4 (wei); NULL when none
    max_fee_p90             NUMERIC,
    max_fee_p99             NUMERIC,
    max_priority_p50        NUMERIC,
    max_priority_p90        NUMERIC,
    max_priority_p99        NUMERIC,
    width_count             BIGINT   NOT NULL DEFAULT 0,  -- M7 (both auth bounds present)
    width_p50_s             DOUBLE PRECISION,
    width_p90_s             DOUBLE PRECISION,
    width_p99_s             DOUBLE PRECISION,
    overprov_count          BIGINT   NOT NULL DEFAULT 0,  -- M9 (gas_limit > 0)
    overprov_ratio_p50      DOUBLE PRECISION,             -- gas_used / gas_limit
    overprov_ratio_p90      DOUBLE PRECISION,
    overprov_ratio_p99      DOUBLE PRECISION,
    tx_value_nonzero_count  BIGINT   NOT NULL DEFAULT 0,  -- M11
    dup_auth_nonce_count    BIGINT   NOT NULL DEFAULT 0,  -- M12
    same_block_replay_count BIGINT   NOT NULL DEFAULT 0,  -- M12
    PRIMARY KEY (window_name, membership)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- M3 batch mechanics: payments-per-tx histogram. max_batch_size is the per-window
-- max, repeated on every bucket row (denormalized constant).
CREATE TABLE IF NOT EXISTS metrics_mechanics_batch_v2 (
    window_name         TEXT     NOT NULL,
    batch_bucket        TEXT     NOT NULL,                -- '1' | '2-10' | '11-100' | '100+'
    methodology_version SMALLINT NOT NULL,
    tx_count            BIGINT   NOT NULL,
    payment_count       BIGINT   NOT NULL,
    max_batch_size      BIGINT   NOT NULL,
    PRIMARY KEY (window_name, batch_bucket)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- M5 block density: payments-per-block stats per window.
CREATE TABLE IF NOT EXISTS metrics_mechanics_block_v2 (
    window_name         TEXT     NOT NULL,
    methodology_version SMALLINT NOT NULL,
    max_per_block       BIGINT   NOT NULL,
    p99_per_block       BIGINT   NOT NULL,
    mean_per_block      DOUBLE PRECISION NOT NULL,
    distinct_blocks     BIGINT   NOT NULL,
    PRIMARY KEY (window_name)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- M2 wrapper/selector mix: top-15 (method_selector, settlement_kind) per
-- (window, membership) by txn_count. Label-agnostic — the page maps hex→names.
CREATE TABLE IF NOT EXISTS metrics_mechanics_selector_v2 (
    window_name         TEXT          NOT NULL,
    membership          TEXT          NOT NULL,           -- 'known' | 'unknown' | 'all'
    rank                INTEGER       NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    selector_hex        TEXT          NOT NULL,
    settlement_kind     TEXT          NOT NULL,
    txn_count           BIGINT        NOT NULL,
    volume_usdc         NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (window_name, membership, rank)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_mechanics_selector_v2;
DROP TABLE IF EXISTS metrics_mechanics_block_v2;
DROP TABLE IF EXISTS metrics_mechanics_batch_v2;
DROP TABLE IF EXISTS metrics_mechanics_window_v2;
-- +goose StatementEnd
