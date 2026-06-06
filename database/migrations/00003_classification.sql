-- +goose Up
-- +goose StatementBegin
-- Methodology changelog: one row per classification version. The versioned
-- views (payment_classified_vN) read dimension rows scoped to their version.
CREATE TABLE IF NOT EXISTS methodology_version (
    version    INTEGER PRIMARY KEY,
    summary    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Facilitator allowlist (tx.from). Presence => the payment counts as agentic x402.
-- Version-stamped: a row is visible to view vN when since_version <= N and
-- (until_version IS NULL OR until_version > N). label/source are enrichment slots
-- so x402scan names can be grafted on later without a schema change.
CREATE TABLE IF NOT EXISTS facilitator_allowlist (
    chain         TEXT        NOT NULL DEFAULT 'base',
    address       TEXT        NOT NULL,           -- lowercased tx.from
    source        TEXT        NOT NULL,           -- 'empirical' | 'x402scan' | 'manual'
    label         TEXT        NULL,               -- human name (enrichment, nullable)
    since_version INTEGER     NOT NULL,
    until_version INTEGER     NULL,               -- NULL = still active
    note          TEXT        NULL,
    added_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chain, address, since_version)
);
-- +goose StatementEnd

-- +goose StatementBegin
-- Contamination denylist (tx.to / called_contract). Presence => NOT agentic
-- (batch/treasury/AA infra that emits USDC AuthorizationUsed). Denylist wins over
-- the allowlist in the view. Ledger, do not delete (measuring the fake is the product).
CREATE TABLE IF NOT EXISTS contamination_denylist (
    chain           TEXT        NOT NULL DEFAULT 'base',
    called_contract TEXT        NOT NULL,         -- lowercased tx.to
    label           TEXT        NOT NULL,
    selector        TEXT        NULL,             -- representative method_selector
    reason          TEXT        NOT NULL,
    confidence      TEXT        NOT NULL,         -- 'confirmed' (known identity) | 'shape' (economic shape only)
    since_version   INTEGER     NOT NULL,
    until_version   INTEGER     NULL,
    added_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chain, called_contract, since_version)
);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO methodology_version (version, summary) VALUES
 (1, 'Attribution turnstile v1: agentic / contested / contamination via tx.from allowlist + tx.to denylist. Empirical allowlist; 6-contract denylist.')
ON CONFLICT (version) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
-- Denylist v1 seed. Confirmed from the 2026-06-06 Jan-Jun analysis (20.6M rows);
-- removes $412.4M = 92% of period volume. See docs/methodology/classification-v1.md.
INSERT INTO contamination_denylist (chain, called_contract, label, selector, reason, confidence, since_version) VALUES
 ('base','0x887749abb233682aa7d5594a54659c51501445b1','SimpleFiatTokenUtil',      '0xcccbb34c','batchTransferWithAuthorization payroll/disperse batch; $339.8M, avg $2,470, 95% whale-$','confirmed',1),
 ('base','0xa757c9421a5d38f5b0402c41abc76332e164a413','Batch settlement util',    '0x7f3e838c','7,896 payees x 4,927 payers, $3.0M max single transfer, 96% whale-$; $48.9M',           'shape',    1),
 ('base','0x5ff137d4b0fdcd49dca30c7cf57e578a026d2789','ERC-4337 EntryPoint v0.6', '0x1fad948c','handleOps AA bundles; gas-payer is a bundler, never an x402 facilitator; $18.4M',        'confirmed',1),
 ('base','0x0000000071727de22e5e9d8baf0edac6f37da032','ERC-4337 EntryPoint v0.7', '0x765e827f','handleOps AA bundles (canonical v0.7 address); $3.1M',                                  'confirmed',1),
 ('base','0xca11bde05977b3631167028862be2a173976ca11','Multicall3',               '0x82ad56cb','generic aggregator (deterministic deploy), 97.8% dust-spam batching, not a settlement path; $1.75M','confirmed',1),
 ('base','0x0de1afc04a6ff8d12e27f942d7903b07cc58fec4','Whale settlement',         '0xae12766d','157 payees x 157 payers, $275k max, 95% whale-$; $0.46M',                               'shape',    1)
ON CONFLICT (chain, called_contract, since_version) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS contamination_denylist;
DROP TABLE IF EXISTS facilitator_allowlist;
DROP TABLE IF EXISTS methodology_version;
-- +goose StatementEnd
