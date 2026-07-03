-- +goose Up
-- Anatomy v2 entity tables: per-address aggregates rebuilt offline by
-- `anatomy rollup`, plus the identity signal tier. All aggregate tables carry
-- facilitator_known as a dimension so the UI lens is a WHERE clause.
-- The leaderboard is the exception: rankings are not mergeable across the
-- boolean, so it precomputes one ranking per lens ('known' | 'all').

CREATE TABLE entity_edge_v1 (
    chain               TEXT           NOT NULL,
    payer               TEXT           NOT NULL,
    payee               TEXT           NOT NULL,
    facilitator_known   BOOLEAN        NOT NULL,
    txn_count           BIGINT         NOT NULL,
    volume_usdc         NUMERIC(38,6)  NOT NULL,
    first_seen          TIMESTAMPTZ    NOT NULL,
    last_seen           TIMESTAMPTZ    NOT NULL,
    methodology_version SMALLINT       NOT NULL,
    PRIMARY KEY (chain, payer, payee, facilitator_known)
);
CREATE INDEX idx_entity_edge_v1_payee ON entity_edge_v1 (chain, payee, facilitator_known);

CREATE TABLE facilitator_edge_v1 (
    chain               TEXT           NOT NULL,
    facilitator         TEXT           NOT NULL,
    counterparty_role   TEXT           NOT NULL CHECK (counterparty_role IN ('payer','payee')),
    counterparty        TEXT           NOT NULL,
    facilitator_known   BOOLEAN        NOT NULL,
    txn_count           BIGINT         NOT NULL,
    volume_usdc         NUMERIC(38,6)  NOT NULL,
    first_seen          TIMESTAMPTZ    NOT NULL,
    last_seen           TIMESTAMPTZ    NOT NULL,
    methodology_version SMALLINT       NOT NULL,
    PRIMARY KEY (chain, facilitator, counterparty_role, counterparty, facilitator_known)
);
CREATE INDEX idx_facilitator_edge_v1_counterparty
    ON facilitator_edge_v1 (chain, counterparty, counterparty_role, facilitator_known);

CREATE TABLE entity_day_v1 (
    chain               TEXT           NOT NULL,
    address             TEXT           NOT NULL,
    role                TEXT           NOT NULL CHECK (role IN ('payer','payee','facilitator')),
    day                 DATE           NOT NULL,
    facilitator_known   BOOLEAN        NOT NULL,
    txn_count           BIGINT         NOT NULL,
    volume_usdc         NUMERIC(38,6)  NOT NULL,
    methodology_version SMALLINT       NOT NULL,
    PRIMARY KEY (chain, address, role, day, facilitator_known)
);

CREATE TABLE entity_price_point_v1 (
    chain                  TEXT           NOT NULL,
    address                TEXT           NOT NULL,
    role                   TEXT           NOT NULL CHECK (role IN ('payer','payee','facilitator')),
    facilitator_known      BOOLEAN        NOT NULL,
    amount_usdc            NUMERIC(38,6)  NOT NULL,
    txn_count              BIGINT         NOT NULL,
    amount_rank            SMALLINT       NOT NULL,
    total_distinct_amounts BIGINT         NOT NULL,
    methodology_version    SMALLINT       NOT NULL,
    PRIMARY KEY (chain, address, role, facilitator_known, amount_usdc)
);

CREATE TABLE entity_leaderboard_v1 (
    window_name             TEXT           NOT NULL CHECK (window_name IN ('7d','30d','all')),
    role                    TEXT           NOT NULL CHECK (role IN ('payer','payee')),
    lens                    TEXT           NOT NULL CHECK (lens IN ('known','all')),
    address                 TEXT           NOT NULL,
    txn_count               BIGINT         NOT NULL,
    volume_usdc             NUMERIC(38,6)  NOT NULL,
    distinct_counterparties BIGINT         NOT NULL,
    first_seen              TIMESTAMPTZ    NOT NULL,
    last_seen               TIMESTAMPTZ    NOT NULL,
    methodology_version     SMALLINT       NOT NULL,
    PRIMARY KEY (window_name, role, lens, address)
);

-- Identity signal tier: one row per observed signal; raw payload kept in meta.
-- Sources v1: 'manual' (data/entity-labels.json). Later: 'catalog', 'erc8004',
-- 'basename' (Plan D collectors insert rows; no schema change).
CREATE TABLE entity_signal (
    chain      TEXT        NOT NULL,
    address    TEXT        NOT NULL,
    source     TEXT        NOT NULL,
    kind       TEXT        NOT NULL,
    value      TEXT        NOT NULL,
    url        TEXT,
    confidence REAL,
    meta       JSONB,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chain, address, source, kind)
);

-- Single-row stamp; source of the UI "data as of" chip.
CREATE TABLE anatomy_meta (
    id                  SMALLINT    PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    data_max_day        DATE,
    built_at            TIMESTAMPTZ NOT NULL,
    methodology_version SMALLINT    NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS anatomy_meta;
DROP TABLE IF EXISTS entity_signal;
DROP TABLE IF EXISTS entity_leaderboard_v1;
DROP TABLE IF EXISTS entity_price_point_v1;
DROP TABLE IF EXISTS entity_day_v1;
DROP TABLE IF EXISTS facilitator_edge_v1;
DROP TABLE IF EXISTS entity_edge_v1;
