-- +goose Up
-- +goose StatementBegin
-- entity_txn_bucket: bucket an entity's lifetime txn_count into a fixed tier.
-- IMMUTABLE so it can be used in GROUP BY. Mirrors amount_band (migration 00009).
CREATE OR REPLACE FUNCTION entity_txn_bucket(n bigint) RETURNS text AS $$
  SELECT CASE
    WHEN n <= 1      THEN '1'
    WHEN n <= 10     THEN '2-10'
    WHEN n <= 100    THEN '11-100'
    WHEN n <= 1000   THEN '101-1k'
    WHEN n <= 100000 THEN '1k-100k'
    ELSE '100k+'
  END
$$ LANGUAGE sql IMMUTABLE;
-- +goose StatementEnd

-- +goose StatementBegin
-- entity_rank_v1: top entities per (window, role). Stores the UNION of
-- top-150-by-volume and top-150-by-txns, each row carrying all measures so a
-- page renders either leaderboard by sorting the union client-side. Rebuilt by
-- `publisher rollup` (RebuildEntities); never written by the collector.
CREATE TABLE IF NOT EXISTS entity_rank_v1 (
    window_name             TEXT          NOT NULL,  -- '7d' | '30d' | 'all'
    role                    TEXT          NOT NULL,  -- 'payee' | 'payer'
    address                 TEXT          NOT NULL,
    volume_usdc             NUMERIC(38,6) NOT NULL,
    txn_count               BIGINT        NOT NULL,
    distinct_counterparties BIGINT        NOT NULL,  -- distinct payers (payee) / payees (payer)
    distinct_amounts        BIGINT        NOT NULL,
    known_volume_usdc       NUMERIC(38,6) NOT NULL,  -- volume via allowlisted facilitators
    first_seen              TIMESTAMPTZ   NOT NULL,
    last_seen               TIMESTAMPTZ   NOT NULL,
    methodology_version     SMALLINT      NOT NULL,
    PRIMARY KEY (window_name, role, address)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- entity_buckets_v1: Y2 activity histogram — entity counts per txn-count bucket.
CREATE TABLE IF NOT EXISTS entity_buckets_v1 (
    window_name         TEXT          NOT NULL,
    role                TEXT          NOT NULL,
    bucket              TEXT          NOT NULL,  -- entity_txn_bucket values
    entity_count        BIGINT        NOT NULL,
    txn_sum             BIGINT        NOT NULL,
    volume_sum          NUMERIC(38,6) NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    PRIMARY KEY (window_name, role, bucket)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- entity_concentration_v1: P11/E9 — totals + top-10/100 share per (window, role).
CREATE TABLE IF NOT EXISTS entity_concentration_v1 (
    window_name         TEXT          NOT NULL,
    role                TEXT          NOT NULL,
    total_entities      BIGINT        NOT NULL,
    total_volume        NUMERIC(38,6) NOT NULL,
    total_txns          BIGINT        NOT NULL,
    top10_volume        NUMERIC(38,6) NOT NULL,
    top10_txns          BIGINT        NOT NULL,
    top100_volume       NUMERIC(38,6) NOT NULL,
    methodology_version SMALLINT      NOT NULL,
    PRIMARY KEY (window_name, role)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS entity_concentration_v1;
DROP TABLE IF EXISTS entity_buckets_v1;
DROP TABLE IF EXISTS entity_rank_v1;
DROP FUNCTION IF EXISTS entity_txn_bucket(bigint);
-- +goose StatementEnd
