-- payment_classified_v1 — attribution turnstile, methodology version 1.
--
-- Labels every payments row agentic / contested / contamination from identity:
--   contamination  tx.to (called_contract) is in the denylist  (wins over everything)
--   agentic        tx.from (facilitator) is in the allowlist
--   contested      neither — real-looking x402 on USDC with no known facilitator
--
-- REPRODUCIBILITY HAZARD (known, deferred): uses `SELECT p.*` — a future
-- payments column silently reshapes this "frozen" view. Enumerate columns in the
-- next vN. See docs/notes/2026-07-06-code-review-findings.md (SQL-1).
--
-- Reads only dimension rows visible at version 1, so this view's output is frozen
-- even after v2 rows are appended. The raw payments row is never rewritten; this
-- supersedes the hardcoded protocol='x402' only at read time.
-- Applied by database/init/init-db.sh after migrations.

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
