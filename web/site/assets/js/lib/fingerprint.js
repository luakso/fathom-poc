// Payee/payer fingerprint: separates services from sinks, fleets and OTC desks
// using only leaderboard fields (txns × distinct counterparties × distinct
// amounts × volume). Pure — no DOM, no globals. All thresholds live here so
// tuning is a one-file edit.

export const FP = {
  OTC_MAX_TXNS: 20,        // OTC: few transfers...
  OTC_MIN_AVG: 10000,      // ...each very large ($)
  DUST_MAX_AVG: 0.01,      // sink: average payment is dust ($)
  MECH_MIN_TXNS: 1000,     // sink/fleet: mechanical scale (txns)
  FLEET_MAX_AMOUNTS: 2,    // fleet: ~one fixed price
  SERVICE_MIN_CPARTIES: 50,// service: many distinct customers
  SERVICE_MIN_AMOUNTS: 10, // service: varied pricing
};

export const CLASSES = {
  otc:     { label: "OTC desk",     color: "var(--contested)", blurb: "few large transfers to ~one counterparty" },
  sink:    { label: "dust sink",    color: "var(--contam)",    blurb: "absorbs micro-payment spam" },
  fleet:   { label: "fleet target", color: "var(--accent)",    blurb: "mechanical fixed-price rail" },
  service: { label: "service",      color: "var(--agentic)",   blurb: "many distinct customers, varied prices" },
  tail:    { label: "long-tail",    color: "var(--dim)",       blurb: "small or ambiguous" },
};

// classify: leaderboard row -> class key. First matching rule wins.
export function classify(row) {
  const T = row.txn_count || 0;
  const C = row.distinct_counterparties || 0;
  const A = row.distinct_amounts || 0;
  const V = parseFloat(row.volume_usdc) || 0;
  const avg = T > 0 ? V / T : 0;
  if (T <= FP.OTC_MAX_TXNS && avg >= FP.OTC_MIN_AVG) return "otc";
  if (avg <= FP.DUST_MAX_AVG && T >= FP.MECH_MIN_TXNS) return "sink";
  if (A <= FP.FLEET_MAX_AMOUNTS && T >= FP.MECH_MIN_TXNS) return "fleet";
  if (C >= FP.SERVICE_MIN_CPARTIES && A >= FP.SERVICE_MIN_AMOUNTS) return "service";
  return "tail";
}
