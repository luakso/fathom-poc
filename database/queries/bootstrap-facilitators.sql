-- Bootstrap the facilitator allowlist (methodology v1) from our own rows.
--
-- Run AFTER the December backfill completes. This is read-only; it derives
-- candidate facilitators (tx.from) that have genuine agentic-band activity and do
-- NOT route through any denylisted contract. Hand-review the output (drop obvious
-- dust-spam batchers that use a non-denylisted wrapper), then write the surviving
-- addresses into 00004_allowlist_seed_v1.sql with source='empirical', since_version=1.
--
-- Requires migration 00003 (contamination_denylist) applied first.
-- Floor and band are tunable — see docs/methodology/classification-v1.md.

SELECT
    p.facilitator,
    count(*)                                                  AS txns,
    count(*) FILTER (WHERE amount_usdc BETWEEN 0.001 AND 100) AS agentic_band_txns,
    round(sum(amount_usdc), 0)                                AS total_usdc,
    round(sum(amount_usdc) FILTER (WHERE amount_usdc BETWEEN 0.001 AND 100), 0) AS agentic_band_usdc,
    count(DISTINCT p.payee)                                   AS distinct_payees
FROM payments p
WHERE p.chain = 'base'
  AND NOT EXISTS (
      SELECT 1 FROM contamination_denylist d
      WHERE d.chain = p.chain
        AND d.called_contract = p.called_contract
        AND d.since_version <= 1 AND (d.until_version IS NULL OR d.until_version > 1)
  )
GROUP BY p.facilitator
HAVING count(*) FILTER (WHERE amount_usdc BETWEEN 0.001 AND 100) >= 100   -- floor (tunable)
ORDER BY agentic_band_txns DESC;
