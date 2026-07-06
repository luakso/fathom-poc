-- entity_identity_v1: the single identity-resolution surface for anatomy.
-- Merges entity_signal label-ish rows with facilitator allowlist labels and
-- picks one display identity per (chain, address) by source precedence:
--   manual > catalog > erc8004 > basename > allowlist.
-- Methodology v1 pins allowlist rows to allowlist version 1 (same convention
-- as payment_x402_v1). New precedence rules => entity_identity_v2.sql.
CREATE OR REPLACE VIEW entity_identity_v1 AS
WITH signals AS (
    SELECT chain, address, source, value AS label, url
    FROM entity_signal
    WHERE kind IN ('label', 'name', 'endpoint')
    UNION ALL
    SELECT chain, address, 'allowlist' AS source, label, NULL AS url
    FROM facilitator_allowlist
    WHERE label IS NOT NULL
      AND since_version <= 1
      AND (until_version IS NULL OR until_version > 1)
),
ranked AS (
    SELECT chain, address, source, label, url,
           row_number() OVER (
               PARTITION BY chain, address
               ORDER BY CASE source
                   WHEN 'manual'    THEN 1
                   WHEN 'catalog'   THEN 2
                   WHEN 'erc8004'   THEN 3
                   WHEN 'basename'  THEN 4
                   WHEN 'allowlist' THEN 5
                   ELSE 6
               END, source, label, url NULLS LAST
           ) AS rn
           -- url is the final tiebreak so the picked identity is fully
           -- deterministic across rebuilds. Without it, two signals sharing
           -- source+label leave the emitted url up to Postgres row order.
           -- This is a determinism fix, not a precedence change (the picked
           -- label/source is unaffected), so it stays in v1 per this file's
           -- own "new PRECEDENCE rules => v2" policy.
    FROM signals
)
SELECT chain, address, source, label, url, 1 AS methodology_version
FROM ranked
WHERE rn = 1;
