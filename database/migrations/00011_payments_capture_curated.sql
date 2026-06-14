-- +goose Up
-- +goose StatementBegin
-- v2 curated capture fields. All derivable from data already in the HyperSync
-- backfill stream (no new field-selection columns). Every ADD is metadata-only:
-- the NOT NULL pair carries a constant DEFAULT (no table rewrite on PG11+), the
-- rest are nullable. payer_account_type is created here for schema completeness
-- but is populated by a later plan (eth_getCode account-state pass).
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS settlement_kind     TEXT        NOT NULL DEFAULT 'transfer',
    ADD COLUMN IF NOT EXISTS self_settled        BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS valid_after         TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS valid_before        TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS input_calldata      BYTEA       NULL,
    ADD COLUMN IF NOT EXISTS block_hash          TEXT        NULL,
    ADD COLUMN IF NOT EXISTS transaction_index   INTEGER     NULL,
    ADD COLUMN IF NOT EXISTS token_decimals      SMALLINT    NULL,
    ADD COLUMN IF NOT EXISTS token_symbol        TEXT        NULL,
    ADD COLUMN IF NOT EXISTS payer_account_type  TEXT        NULL;

-- payment_x402_v1 — the v2 read view. Every stored row is in-set (x402), so the
-- only label is facilitator_known (allowlist join, version 1). Kept byte-
-- identical to database/views/payment_x402_v1.sql, which init-db re-applies.
CREATE OR REPLACE VIEW payment_x402_v1 AS
WITH allow AS (
    SELECT chain, address
    FROM facilitator_allowlist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
)
SELECT
    p.*,
    (a.address IS NOT NULL) AS facilitator_known,
    1 AS methodology_version
FROM payments p
LEFT JOIN allow a ON a.chain = p.chain AND a.address = p.facilitator;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP VIEW IF EXISTS payment_x402_v1;
ALTER TABLE payments
    DROP COLUMN IF EXISTS settlement_kind,
    DROP COLUMN IF EXISTS self_settled,
    DROP COLUMN IF EXISTS valid_after,
    DROP COLUMN IF EXISTS valid_before,
    DROP COLUMN IF EXISTS input_calldata,
    DROP COLUMN IF EXISTS block_hash,
    DROP COLUMN IF EXISTS transaction_index,
    DROP COLUMN IF EXISTS token_decimals,
    DROP COLUMN IF EXISTS token_symbol,
    DROP COLUMN IF EXISTS payer_account_type;
-- +goose StatementEnd
