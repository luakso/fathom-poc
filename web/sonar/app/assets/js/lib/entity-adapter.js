// Loads a payees/payers artifact, validates it, and re-checks the invariants
// the emitter promised: buckets reconcile to concentration totals, and the
// page's concentration equals economy.json's block for this role. The page
// renders nothing it cannot verify; these checks feed the VERIFY LOG.
import { num, numFinite } from "../format.js";
import { fetchJson as fetchDoc } from "./fetch-json.js";

export const USD_TOLERANCE = 0.5; // counts conserve exactly; dollars within $0.50

// Every window an entity artifact ships must carry these sub-sections; a
// truncated artifact that drops one would otherwise deep-crash with a raw
// TypeError. Mirrors adapter.js's REQUIRED_SECTIONS: collect ALL missing keys
// and fail with one named, aggregated error.
const REQUIRED_WINDOW_SECTIONS = ["concentration", "buckets", "leaderboard"];
function validateEntity(d) {
  const missing = [];
  if (!d || typeof d !== "object") { throw new Error("artifact missing sections: data"); }
  if (!d.windows || typeof d.windows !== "object") missing.push("windows");
  else for (const [w, win] of Object.entries(d.windows)) {
    for (const s of REQUIRED_WINDOW_SECTIONS) {
      if (!win || win[s] === undefined) missing.push(`${w}.${s}`);
    }
  }
  if (missing.length) throw new Error(`artifact missing sections: ${missing.join(", ")}`);
}

export function reshapeEntity(doc) {
  const d = doc.data;
  validateEntity(d);
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
    for (const b of win.buckets) {
      bt += numFinite(b.txn_sum,      `${w}.bucket.txn_sum`);
      be += numFinite(b.entity_count, `${w}.bucket.entity_count`);
      bv += numFinite(b.volume_sum,   `${w}.bucket.volume_sum`);
    }
    const cTxns = numFinite(c.total_txns,    `${w}.concentration.total_txns`);
    const cEnts = numFinite(c.total_entities, `${w}.concentration.total_entities`);
    const cVol  = numFinite(c.total_volume,  `${w}.concentration.total_volume`);
    if (bt !== cTxns) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket txns ${bt} ≠ total ${cTxns}` });
    }
    if (be !== cEnts) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket entities ${be} ≠ total ${cEnts}` });
    }
    if (Math.abs(bv - cVol) > USD_TOLERANCE) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} bucket USD off by $${Math.abs(bv - cVol).toFixed(2)}` });
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
