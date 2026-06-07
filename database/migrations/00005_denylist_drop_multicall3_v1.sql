-- +goose Up
-- +goose StatementBegin
-- Drop Multicall3 from the v1 contamination denylist.
--
-- Empirical finding (2026-06-07, full 64.4M-row dataset): unlike the other five
-- denylisted contracts -- which have ZERO known-facilitator tx.from and are
-- definitionally non-x402 (SimpleFiatTokenUtil/batch utils/AA EntryPoints/whale
-- settler) -- Multicall3 carries 99.2% of its txns ($1.70M of $1.76M, avg
-- $0.66/txn) from known x402 facilitators. That is textbook sanctioned
-- `batch-settlement` (facilitators aggregating micropayments through a generic
-- aggregator), not contamination. Multicall3 was originally denylisted on dust
-- *shape* ("97.8% dust-spam"), which conflates the authenticity axis with
-- attribution. The denylist must hold only confirmed non-x402 *identities*.
--
-- Effect via the existing view precedence (denylist > allowlist > contested):
--   Multicall3 + allowlisted tx.from -> agentic   (2.59M txns, ~$1.70M)
--   Multicall3 + unknown tx.from     -> contested  (~21k txns, ~$55k)
-- No view change needed; removing the dimension row is sufficient.
DELETE FROM contamination_denylist
 WHERE chain = 'base'
   AND called_contract = '0xca11bde05977b3631167028862be2a173976ca11'
   AND since_version = 1;

UPDATE methodology_version
   SET summary = 'Attribution turnstile v1: agentic / contested / contamination via tx.from allowlist + tx.to denylist. Allowlist = 112 x402scan facilitator addresses (identity); 5-contract denylist (Multicall3 dropped: facilitator batch-settlement, not contamination).'
 WHERE version = 1;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
INSERT INTO contamination_denylist (chain, called_contract, label, selector, reason, confidence, since_version) VALUES
 ('base','0xca11bde05977b3631167028862be2a173976ca11','Multicall3','0x82ad56cb','generic aggregator (deterministic deploy), 97.8% dust-spam batching, not a settlement path; $1.75M','confirmed',1)
ON CONFLICT (chain, called_contract, since_version) DO NOTHING;

UPDATE methodology_version
   SET summary = 'Attribution turnstile v1: agentic / contested / contamination via tx.from allowlist + tx.to denylist. Allowlist = 112 x402scan facilitator addresses (identity); 6-contract denylist.'
 WHERE version = 1;
-- +goose StatementEnd
