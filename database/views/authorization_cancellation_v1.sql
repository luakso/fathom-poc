-- authorization_cancellation_v1 — v2 read view, methodology version 1.
--
-- ERC-3009 AuthorizationCanceled events (payer abandoned a signed authorization
-- before use). All USDC cancellations are stored losslessly; facilitator_known
-- labels whether the cancel was submitted by a known facilitator (tx.from on the
-- allowlist), mirroring payment_x402_v1. A canceled authorization was never used,
-- so transaction_from is the only available linkage. Applied by init-db after
-- migrations; kept byte-identical to the view DDL in migration 00013.

CREATE OR REPLACE VIEW authorization_cancellation_v1 AS
WITH allow AS (
    SELECT chain, address
    FROM facilitator_allowlist
    WHERE since_version <= 1 AND (until_version IS NULL OR until_version > 1)
)
SELECT
    c.*,
    (a.address IS NOT NULL) AS facilitator_known,
    1 AS methodology_version
FROM authorization_cancellations c
LEFT JOIN allow a ON a.chain = c.chain AND a.address = c.transaction_from;
