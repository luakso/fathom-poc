-- payment_x402_v1 — v2 read view, methodology version 1.
--
-- In v2 every stored payments row is in-set (x402 by facilitator linkage), so
-- there is no agentic/contested/contamination split — the only label is
-- facilitator_known (tx.from is a known facilitator). settlement_kind and
-- self_settled are columns on the row itself. Applied by init-db after migrations.

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
