-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS payments (
    -- Identity / PK (multicall-aware)
    chain               TEXT          NOT NULL,
    tx_hash             TEXT          NOT NULL,
    log_index           INTEGER       NOT NULL,
    PRIMARY KEY (chain, tx_hash, log_index),

    -- Position on chain
    block_number        BIGINT        NOT NULL,
    block_timestamp     TIMESTAMPTZ   NOT NULL,

    -- Observation metadata
    observed_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    source              TEXT          NOT NULL,
    protocol            TEXT          NOT NULL,

    -- Payment principals (lowercased)
    facilitator         TEXT          NOT NULL,
    payer               TEXT          NOT NULL,
    payee               TEXT          NOT NULL,
    payee_service_id    BIGINT        NULL,

    -- Amount (exact + convenience; NUMERIC, never f64)
    asset               TEXT          NOT NULL,
    token_address       TEXT          NOT NULL,
    amount_raw          NUMERIC(78,0) NOT NULL,
    amount_usdc         NUMERIC(38,6) NOT NULL,
    asset_usd_at_time   NUMERIC(20,8) NOT NULL,

    -- EIP-3009 authorization id
    auth_nonce          BYTEA         NOT NULL,

    -- Routing metadata
    method_selector     BYTEA         NOT NULL,
    called_contract     TEXT          NOT NULL,
    tx_type             SMALLINT      NOT NULL,
    tx_nonce            BIGINT        NOT NULL,

    -- Gas economics
    gas_used            BIGINT        NOT NULL,
    effective_gas_price NUMERIC(78,0) NOT NULL,
    gas_cost_wei        NUMERIC(78,0) NOT NULL,
    base_fee_per_gas    NUMERIC(78,0) NULL
);

CREATE INDEX IF NOT EXISTS idx_payments_block       ON payments(chain, block_number);
CREATE INDEX IF NOT EXISTS idx_payments_timestamp   ON payments(chain, block_timestamp);
CREATE INDEX IF NOT EXISTS idx_payments_facilitator ON payments(facilitator);
CREATE INDEX IF NOT EXISTS idx_payments_payer       ON payments(payer);
CREATE INDEX IF NOT EXISTS idx_payments_payee       ON payments(payee);

CREATE TABLE IF NOT EXISTS collector_cursor (
    collector_name  TEXT PRIMARY KEY,
    last_block      BIGINT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS collector_cursor;
-- +goose StatementEnd
