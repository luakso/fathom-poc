-- payment_x402_v1 — v2 read view, methodology version 1.
--
-- In v2 every stored payments row is in-set (x402 by facilitator linkage), so
-- there is no agentic/contested/contamination split — the only label is
-- facilitator_known (tx.from is a known facilitator). settlement_kind and
-- self_settled are columns on the row itself. Applied by init-db after migrations.
--
-- REPRODUCIBILITY HAZARD (known, deferred): the `SELECT p.*` below means any
-- future `ALTER TABLE payments ADD COLUMN` silently changes this frozen view's
-- projected columns and their order — a methodology view claimed immutable is
-- then mutated by unrelated substrate changes. Fixing this properly means a new
-- payment_x402_v2 that enumerates its columns explicitly (per the migrations-vs-
-- views rule in CLAUDE.md) plus a methodology_version bump and a re-rollup/
-- re-emit — a deliberate prod operation, not an in-place edit of a shipped v1
-- view. Until then: do NOT rely on this view's column set being frozen, and
-- enumerate columns explicitly in the next vN. See
-- docs/notes/2026-07-06-code-review-findings.md (SQL-1).

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
