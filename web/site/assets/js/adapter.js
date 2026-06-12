// Loads the canonical economy.json and reshapes it into the view-model the
// renderers consume. Also the integrity gate: version guard + client-side
// conservation re-check, so the status bar's glyphs are earned, not painted.
import { num } from "./format.js";

const MONTH_NAME = m => new Date(m + "-01T00:00:00Z").toLocaleString("en-US", { month:"short", timeZone:"UTC" });

// reshape: artifact shape -> view-model (exported separately so it can be
// equivalence-checked against a fixture without a DOM or network).
export function reshape(doc){
  const d = doc.data;
  return {
    meta: {
      methodology_version: doc.methodology_version,
      generated_at: doc.generated_at,
      data_through_day: doc.data_through_day,
    },
    windows: d.windows,
    // compact tape: [day, txn_count, volume rounded to cents]
    daily: d.daily_series.map(p => [p.day, p.txn_count, Math.round(num(p.volume_usdc)*100)/100]),
    monthly: d.monthly_series,
    typical: d.typical_payment,
    // renderers show top-12 price points per window; artifact ships 50
    price_points: Object.fromEntries(
      Object.entries(d.price_points).map(([w, arr]) => [w, arr.slice(0, 12)])
    ),
    gas: d.gas,
    velocity: {
      windows: d.velocity.windows,
      agentic_daily: d.velocity.daily_series
        .filter(p => p.attribution === "agentic")
        .map(p => [p.day, p.max_per_min, p.p99_per_min]),
    },
    claims: d.claims,
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

// checkIntegrity: returns a list of {level:"warn"|"error", msg}. Counts must
// conserve exactly; dollars within $0.50 (display-grade float tolerance).
export function checkIntegrity(view){
  const issues = [];
  if (view.meta.methodology_version !== 1){
    issues.push({ level:"warn", msg:`methodology v${view.meta.methodology_version} — this page was built for v1; review before citing` });
  }
  for (const [w, win] of Object.entries(view.windows)){
    let n = 0, usd = 0;
    for (const m of Object.values(win.by_attribution)){ n += m.txn_count; usd += num(m.volume_usdc); }
    if (n !== win.txn_count){
      issues.push({ level:"error", msg:`conservation ✗ — ${w} attribution txns ${n} ≠ total ${win.txn_count}` });
    }
    if (Math.abs(usd - num(win.volume_usdc)) > 0.5){
      issues.push({ level:"error", msg:`conservation ✗ — ${w} attribution USD off by $${Math.abs(usd - num(win.volume_usdc)).toFixed(2)}` });
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
