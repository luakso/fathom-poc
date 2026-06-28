// Loads a payees/payers artifact, validates it, and re-checks the invariants
// the emitter promised: buckets reconcile to concentration totals, and the
// page's concentration equals economy.json's block for this role. The page
// renders nothing it cannot verify; these checks feed the VERIFY LOG.
import { num } from "../format.js";

export const USD_TOLERANCE = 0.5; // counts conserve exactly; dollars within $0.50

export function reshapeEntity(doc) {
  const d = doc.data;
  return {
    meta: {
      methodology_version: doc.methodology_version,
      generated_at: doc.generated_at,
      data_through_day: doc.data_through_day,
    },
    role: d.role,
    windows: d.windows, // { "7d"|"30d"|"all": { leaderboard, buckets, concentration } }
  };
}

export function checkEntityIntegrity(view) {
  const issues = [];
  if (view.meta.methodology_version !== 1) {
    issues.push({ level: "warn", msg: `methodology v${view.meta.methodology_version} — this page was built for v1; review before citing` });
  }
  for (const [w, win] of Object.entries(view.windows)) {
    const c = win.concentration;
    let bt = 0, be = 0, bv = 0;
    for (const b of win.buckets) { bt += b.txn_sum; be += b.entity_count; bv += num(b.volume_sum); }
    if (bt !== c.total_txns) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket txns ${bt} ≠ total ${c.total_txns}` });
    }
    if (be !== c.total_entities) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket entities ${be} ≠ total ${c.total_entities}` });
    }
    if (Math.abs(bv - num(c.total_volume)) > USD_TOLERANCE) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket USD off by $${Math.abs(bv - num(c.total_volume)).toFixed(2)}` });
    }
  }
  return issues;
}

export function crossCheckEconomy(view, economyDoc) {
  const issues = [];
  const role = view.role;
  const ec = economyDoc && economyDoc.data && economyDoc.data.concentration && economyDoc.data.concentration.windows;
  if (!ec) return [{ level: "warn", msg: "economy.json has no concentration block to cross-check" }];
  for (const [w, win] of Object.entries(view.windows)) {
    const a = win.concentration;
    const b = ec[w] && ec[w][role];
    if (!b) { issues.push({ level: "warn", msg: `no economy concentration for ${role}/${w}` }); continue; }
    if (a.total_txns !== b.total_txns) {
      issues.push({ level: "error", msg: `cross-check ✗ — ${w} ${role} txns ${a.total_txns} ≠ economy ${b.total_txns}` });
    }
    if (Math.abs(num(a.total_volume) - num(b.total_volume)) > USD_TOLERANCE) {
      issues.push({ level: "error", msg: `cross-check ✗ — ${w} ${role} USD off by $${Math.abs(num(a.total_volume) - num(b.total_volume)).toFixed(2)}` });
    }
  }
  return issues;
}

// loadEntity: fetch <role>s.json + economy.json, validate, return view + issues.
// Throws Error with a readable message on any hard failure; caller owns the
// error screen.
export async function loadEntity(role) {
  const file = `./${role}s.json`;
  const view = await fetchDoc(file, d => d && d.data && d.data.windows, "data.windows");
  let economy = null;
  try {
    economy = await fetchDoc("./economy.json", d => d && d.data, "data");
  } catch (e) {
    // economy.json is only needed for the cross-check; degrade to a warning.
    economy = { data: {} };
  }
  const v = reshapeEntity(view);
  const issues = [...checkEntityIntegrity(v), ...crossCheckEconomy(v, economy)];
  return { view: v, issues };
}

async function fetchDoc(url, ok, fieldHint) {
  let res;
  try { res = await fetch(url, { cache: "no-cache" }); }
  catch (e) { throw new Error(`network error fetching ${url}: ${e.message}`); }
  if (!res.ok) throw new Error(`HTTP ${res.status} fetching ${url}`);
  let doc;
  try { doc = await res.json(); }
  catch (e) { throw new Error(`${url} is not valid JSON: ${e.message}`); }
  if (!ok(doc)) throw new Error(`${url} missing expected fields (${fieldHint})`);
  return doc;
}
