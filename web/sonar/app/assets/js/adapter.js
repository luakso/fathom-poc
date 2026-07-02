// Loads the canonical economy.json and reshapes it into the view-model the
// renderers consume. Also the integrity gate: version guard + client-side
// conservation re-check, so the status bar's glyphs are earned, not painted.
import { num, BANDDEF } from "./format.js";

const MONTH_NAME = m => new Date(m + "-01T00:00:00Z").toLocaleString("en-US", { month:"short", timeZone:"UTC" });

// The emitter ships sparse maps: an amount band or window stat exists only
// when the window actually contains rows for it. Renderers and pinners index
// densely, so absent keys are filled with zero entries here at the boundary
// instead of guards in every consumer.
const BAND_KEYS = BANDDEF.map(([k]) => k);
const ZERO_MEASURE  = { txn_count: 0, volume_usdc: "0" };
const ZERO_TYPICAL  = { avg_usdc: "0", median_usdc: "0", txn_count: 0 };
const ZERO_GAS      = { txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null, breakeven_txn_count: 0 };
const ZERO_VELOCITY = { max_per_min: 0 };
const dense = (m, keys, zero) => Object.fromEntries(keys.map(k => [k, (m && m[k]) || { ...zero }]));

// reshape: artifact shape -> view-model (exported separately so it can be
// equivalence-checked against a fixture without a DOM or network).
export function reshape(doc){
  const d = doc.data;
  const winKeys = Object.keys(d.windows);
  return {
    meta: {
      methodology_version: doc.methodology_version,
      generated_at: doc.generated_at,
      data_through_day: doc.data_through_day,
    },
    windows: Object.fromEntries(winKeys.map(w => [w, {
      ...d.windows[w],
      by_band: dense(d.windows[w].by_band, BAND_KEYS, ZERO_MEASURE),
    }])),
    // compact tape: [day, txn_count, volume rounded to cents]
    daily: d.daily_series.map(p => [p.day, p.txn_count, Math.round(num(p.volume_usdc)*100)/100]),
    monthly: d.monthly_series,
    typical: Object.fromEntries(winKeys.map(w => [w, (d.typical_payment[w]) || { ...ZERO_TYPICAL }])),
    // renderers show top-12 price points per window; artifact ships 50
    price_points: Object.fromEntries(winKeys.map(w => [w, (d.price_points[w] || []).slice(0, 12)])),
    gas: {
      ...d.gas,
      windows: Object.fromEntries(winKeys.map(w => {
        const g = d.gas.windows[w] || {};
        return [w, { ...g, by_band: g.by_band || {} }];
      })),
    },
    velocity: {
      windows: Object.fromEntries(winKeys.map(w => [w, d.velocity.windows[w] || { ...ZERO_VELOCITY }])),
      verified_daily: d.velocity.daily_series.map(p => [p.day, p.max_per_min, p.p99_per_min]),
    },
    claims: d.claims || [],
    // E9 concentration (window -> role -> {total_entities,...}); the overview
    // reads payee.total_entities as the verified "active payees" count.
    concentration: d.concentration || { windows: {} },
    // excluded: all-window totals of non-verified transfers, for the overview
    // exclusion sentence. Tolerate absence (older artifacts lack this key).
    excluded: d.excluded || null,
  };
}

// winLabels: derive the "all" label from the data's month range.
export function winLabels(view){
  const months = view.monthly.map(m => m.month);
  const span = months.length
    ? `all data ${MONTH_NAME(months[0])}–${MONTH_NAME(months[months.length-1])}`
    : "all data";
  return { "7d":"trailing 7d", "30d":"trailing 30d", "all":span };
}

// Conservation tolerance shared by the boot gate and the VERIFY LOG panel —
// the two must never disagree about the same sum. Counts conserve exactly;
// dollars within $0.50 (display-grade float tolerance over decimal strings).
export const USD_TOLERANCE = 0.5;

// checkIntegrity: returns a list of {level:"warn"|"error", msg}. Counts must
// conserve exactly; dollars within USD_TOLERANCE.
export function checkIntegrity(view){
  const issues = [];
  if (view.meta.methodology_version !== 1){
    issues.push({ level:"warn", msg:`methodology v${view.meta.methodology_version} — this page was built for v1; review before citing` });
  }
  for (const [w, win] of Object.entries(view.windows)){
    let n = 0, usd = 0;
    for (const m of Object.values(win.by_band)){ n += m.txn_count; usd += num(m.volume_usdc); }
    if (n !== win.txn_count){
      issues.push({ level:"error", msg:`conservation ✗ — ${w} band txns ${n} ≠ total ${win.txn_count}` });
    }
    if (Math.abs(usd - num(win.volume_usdc)) > USD_TOLERANCE){
      issues.push({ level:"error", msg:`conservation ✗ — ${w} band USD off by $${Math.abs(usd - num(win.volume_usdc)).toFixed(2)}` });
    }
  }
  return issues;
}

// loadEconomy: fetch + parse + reshape. Throws Error with a readable message
// on any failure; the caller owns the error screen.
export async function loadEconomy(){
  let res;
  try { res = await fetch("./economy.json", { cache:"no-cache" }); }
  catch (e){ throw new Error("network error: " + e.message); }
  if (!res.ok) throw new Error(`HTTP ${res.status} fetching economy.json`);
  let doc;
  try { doc = await res.json(); }
  catch (e){ throw new Error("economy.json is not valid JSON: " + e.message); }
  if (!doc || !doc.data || !doc.data.windows) throw new Error("economy.json missing expected fields (data.windows)");
  const view = reshape(doc);
  return { view, issues: checkIntegrity(view) };
}
