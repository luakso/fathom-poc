-- +goose Up
-- +goose StatementBegin
-- Make amount_usdc a GENERATED column derived from amount_raw.
--
-- amount_usdc is, by definition, amount_raw / 10^6 (USDC has 6 decimals). It
-- was being computed in Go and written as an independent column, which means it
-- could silently drift from amount_raw on any code change. A generated column
-- makes the database the single source of truth and removes the redundant write.
--
-- We MULTIPLY by 0.000001 rather than divide by 1000000: Postgres numeric
-- division targets ~16 significant digits, so for a large amount_raw the
-- quotient is rounded to scale 0 (dropping the cents) BEFORE the cast can fix
-- it. Multiplication's result scale is the sum of operand scales (0 + 6 = 6),
-- so amount_raw * 0.000001 is exact; the (38,6) cast preserves type and range.
--
-- Postgres has no in-place "convert existing column to generated", so this drops
-- and re-adds the column. ADD COLUMN ... GENERATED ... STORED REWRITES THE TABLE
-- under an ACCESS EXCLUSIVE lock — on the current 64M+ rows this takes minutes
-- and blocks reads/writes for its duration. Run it in a maintenance window with
-- the collector paused. (Postgres lacks VIRTUAL generated columns before 18, so
-- a no-rewrite variant is not available here.)
ALTER TABLE payments DROP COLUMN amount_usdc;
ALTER TABLE payments
    ADD COLUMN amount_usdc NUMERIC(38,6)
    GENERATED ALWAYS AS ((amount_raw * 0.000001)::numeric(38,6)) STORED;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Restore amount_usdc as a plain (non-generated) column, repopulated from
-- amount_raw so the down-migration leaves a consistent table. Writers (store.go
-- at this revision) must resume supplying the value themselves.
ALTER TABLE payments DROP COLUMN amount_usdc;
ALTER TABLE payments ADD COLUMN amount_usdc NUMERIC(38,6) NOT NULL DEFAULT 0;
ALTER TABLE payments ALTER COLUMN amount_usdc DROP DEFAULT;
UPDATE payments SET amount_usdc = (amount_raw * 0.000001)::numeric(38,6);
-- +goose StatementEnd
