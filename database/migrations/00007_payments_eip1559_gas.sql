-- +goose Up
-- +goose StatementBegin
-- Capture the EIP-1559 fee caps the sender BID, alongside what they actually
-- paid (effective_gas_price) and the block floor (base_fee_per_gas).
--
--   effective_gas_price  = min(max_fee_per_gas, base_fee_per_gas + max_priority_fee_per_gas)
--
-- Storing only the effective price discards the sender's intent: how much
-- headroom they left (max_fee - effective) and how hard they tipped
-- (max_priority_fee). That gap is the raw material for facilitator-economics
-- analysis — overpayment, urgency, and cost discipline per facilitator.
--
-- Both are NULL on legacy (type 0) and EIP-2930 (type 1) txs, which carry no
-- 1559 fee market. Nullable ADD COLUMN with no default is metadata-only — no
-- rewrite of the existing 64M+ rows. Existing rows backfill as NULL; re-running
-- the collector over a range repopulates them (idempotent via the PK).
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS max_fee_per_gas          NUMERIC(78,0) NULL,
    ADD COLUMN IF NOT EXISTS max_priority_fee_per_gas NUMERIC(78,0) NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE payments
    DROP COLUMN IF EXISTS max_fee_per_gas,
    DROP COLUMN IF EXISTS max_priority_fee_per_gas;
-- +goose StatementEnd
