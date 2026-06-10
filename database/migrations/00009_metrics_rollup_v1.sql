-- +goose Up
-- +goose StatementBegin
-- amount_band: bucket a USD amount into a fixed, ordered tier. IMMUTABLE so it
-- can be used in the rollup GROUP BY and indexes. Tiers are tunable; a change is
-- a methodology change (rebuild the cube). Bands are lower-inclusive, upper-exclusive.
CREATE OR REPLACE FUNCTION amount_band(usd numeric) RETURNS text AS $$
  SELECT CASE
    WHEN usd < 0.01   THEN 'dust'
    WHEN usd < 1      THEN 'micro'
    WHEN usd < 100    THEN 'small'
    WHEN usd < 1000   THEN 'mid'
    ELSE 'whale'
  END
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
-- metrics_daily_v1: the rollup cube. Grain = one row per
-- (day, chain, facilitator, attribution, amount_band). Measures are additive
-- (sum/max) so any window or filter is a roll-up of these rows. Fully recomputed
-- from payment_classified_v1 by `publisher rollup`; never written by the collector.
CREATE TABLE IF NOT EXISTS metrics_daily_v1 (
    day          DATE          NOT NULL,
    chain        TEXT          NOT NULL,
    facilitator  TEXT          NOT NULL,
    attribution  TEXT          NOT NULL,
    amount_band  TEXT          NOT NULL,
    -- Carried through from the classification view so artifact stamps are
    -- derived from the data, not re-declared in Go. Single-valued per rebuild;
    -- emit asserts that.
    methodology_version SMALLINT NOT NULL,
    txn_count    BIGINT        NOT NULL,
    volume_usdc  NUMERIC(38,6) NOT NULL,
    max_amount_usdc NUMERIC(38,6) NOT NULL,
    PRIMARY KEY (day, chain, facilitator, attribution, amount_band)
);
-- No separate index on (day): the primary key's leading column already serves
-- every day-range scan.
CREATE INDEX IF NOT EXISTS idx_metrics_daily_facilitator ON metrics_daily_v1(facilitator);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics_daily_v1;
DROP FUNCTION IF EXISTS amount_band(numeric);
-- +goose StatementEnd
