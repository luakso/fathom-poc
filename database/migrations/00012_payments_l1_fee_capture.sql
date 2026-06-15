-- +goose Up
-- +goose StatementBegin
-- v2 Plan 2: expanded HyperSync capture — fields that need NEW field-selection
-- columns. l1_fee is the missing half of true settlement cost on an OP-stack L2
-- (true cost = gas_cost_wei + l1_fee); l1_gas_used / l1_gas_price are its
-- components; tx_value is the ETH/wei sent with the tx (~0 for x402); gas_limit
-- (the tx gas LIMIT, distinct from gas_used) vs gas_used is the over-
-- provisioning signal. All nullable, so every ADD is metadata-only (no table
-- rewrite). l1_gas_used is NUMERIC(78,0) (not BIGINT like gas_used/gas_limit) on
-- purpose: it shares the L1 trio's uniform nullable-*big.Int capture path, and it
-- is frequently absent (pre-Ecotone / system txs) where NUMERIC NULL preserves the
-- "no L1 gas" state that a BIGINT uint64 0-default would conflate with a real 0.
-- The dead l1_fee_scalar (Bedrock-only, absent post-Ecotone on Base,
-- live-verified) and the patchy Ecotone scalars are deliberately NOT captured
-- (recoverable via re-backfill). See the update note in
-- docs/superpowers/specs/2026-06-14-x402-entity-substrate-design.md.
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS l1_fee        NUMERIC(78,0) NULL,
    ADD COLUMN IF NOT EXISTS l1_gas_used   NUMERIC(78,0) NULL,
    ADD COLUMN IF NOT EXISTS l1_gas_price  NUMERIC(78,0) NULL,
    ADD COLUMN IF NOT EXISTS tx_value      NUMERIC(78,0) NULL,
    ADD COLUMN IF NOT EXISTS gas_limit     BIGINT        NULL;

-- Both read views are `SELECT p.*, <computed trailing columns>`. Adding the
-- columns above shifts those trailing columns, so a plain CREATE OR REPLACE of
-- the canonical view files (what init-db re-applies) fails with "cannot change
-- name of view column". Drop and recreate both here so their stored column lists
-- match the post-00012 p.* expansion, making init-db's re-apply a no-op (same
-- pattern as 00008 and 00011). Bodies are byte-identical to
-- database/views/payment_classified_v1.sql and database/views/payment_x402_v1.sql.
DROP VIEW IF EXISTS payment_x402_v1;
DROP VIEW IF EXISTS payment_classified_v1;
CREATE OR REPLACE VIEW payment_classified_v1 AS
WITH allow AS (
    SELECT chain, address
    FROM facilitator_allowlist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
),
deny AS (
    SELECT chain, called_contract
    FROM contamination_denylist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
)
SELECT
    p.*,
    CASE
        WHEN d.called_contract IS NOT NULL THEN 'contamination'
        WHEN a.address         IS NOT NULL THEN 'agentic'
        ELSE 'contested'
    END AS attribution,
    1 AS methodology_version
FROM payments p
LEFT JOIN deny  d ON d.chain = p.chain AND d.called_contract = p.called_contract
LEFT JOIN allow a ON a.chain = p.chain AND a.address = p.facilitator;

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
-- Drop both p.*-based views before dropping columns, then recreate both at their
-- pre-00012 (post-00011) column shape — byte-identical to the canonical files.
DROP VIEW IF EXISTS payment_x402_v1;
DROP VIEW IF EXISTS payment_classified_v1;
ALTER TABLE payments
    DROP COLUMN IF EXISTS l1_fee,
    DROP COLUMN IF EXISTS l1_gas_used,
    DROP COLUMN IF EXISTS l1_gas_price,
    DROP COLUMN IF EXISTS tx_value,
    DROP COLUMN IF EXISTS gas_limit;
CREATE OR REPLACE VIEW payment_classified_v1 AS
WITH allow AS (
    SELECT chain, address
    FROM facilitator_allowlist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
),
deny AS (
    SELECT chain, called_contract
    FROM contamination_denylist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
)
SELECT
    p.*,
    CASE
        WHEN d.called_contract IS NOT NULL THEN 'contamination'
        WHEN a.address         IS NOT NULL THEN 'agentic'
        ELSE 'contested'
    END AS attribution,
    1 AS methodology_version
FROM payments p
LEFT JOIN deny  d ON d.chain = p.chain AND d.called_contract = p.called_contract
LEFT JOIN allow a ON a.chain = p.chain AND a.address = p.facilitator;
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
