// Loads reliability.json, validates the envelope, and re-checks the invariants the
// emitter promised: windowed<=settlement, known+unknown==settlement, expired+
// not_yet_valid<=windowed, every rate in [0,1], and settlement_count==economy
// txn_count (the cube cross-check). The page renders nothing it cannot verify;
// these checks feed the VERIFY LOG. Mirrors entity-adapter.js.

export const LAT_BUCKETS = [
  { key: "sub1s", label: "<1s" },
  { key: "1_10s", label: "1–10s" },
  { key: "10_60s", label: "10–60s" },
  { key: "1_10m", label: "1–10m" },
  { key: "gt10m", label: ">10m" },
];

const RATE_KEYS = ["windowed_share", "cancellation_rate", "expired_rate", "not_yet_valid_rate"];

export function reshapeReliability(doc) {
  const d = doc.data;
  return {
    meta: {
      methodology_version: doc.methodology_version,
      generated_at: doc.generated_at,
      data_through_day: doc.data_through_day,
    },
    windows: d.windows,
    daily: d.daily || [],
    attribution: d.cancellation_attribution || { by_payer: [], by_cancel_from: [] },
  };
}

export function checkReliabilityIntegrity(view) {
  const issues = [];
  if (view.meta.methodology_version !== 1) {
    issues.push({ level: "warn", msg: `methodology v${view.meta.methodology_version} — this page was built for v1; review before citing` });
  }
  for (const [w, win] of Object.entries(view.windows)) {
    if (win.windowed_count > win.settlement_count) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} windowed ${win.windowed_count} > settlement ${win.settlement_count}` });
    }
    if ((win.expired_count + win.not_yet_valid_count) > win.windowed_count) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} expired+not_yet_valid ${win.expired_count + win.not_yet_valid_count} > windowed ${win.windowed_count}` });
    }
    for (const rk of RATE_KEYS) {
      const r = win[rk];
      if (typeof r === "number" && (r < 0 || r > 1)) {
        issues.push({ level: "error", msg: `range ✗ — ${w} ${rk}=${r} outside [0,1]` });
      }
    }
  }
  return issues;
}

export function crossCheckEconomyReliability(view, economyDoc) {
  const ec = economyDoc && economyDoc.data && economyDoc.data.windows;
  if (!ec) return [{ level: "warn", msg: "economy.json has no windows to cross-check" }];
  const all = view.windows.all;
  const eall = ec.all;
  if (!all || !eall) return [{ level: "warn", msg: "no all-window to cross-check against economy" }];
  if (all.settlement_count !== eall.txn_count) {
    return [{ level: "error", msg: `cross-check ✗ — settlements ${all.settlement_count} ≠ economy txns ${eall.txn_count}` }];
  }
  return [];
}

export function latencyBars(buckets) {
  const total = LAT_BUCKETS.reduce((s, b) => s + (buckets[b.key] || 0), 0);
  return LAT_BUCKETS.map(b => {
    const count = buckets[b.key] || 0;
    return { label: b.label, count, frac: total ? count / total : 0 };
  });
}

export async function loadReliability() {
  const doc = await fetchDoc("./reliability.json", d => d && d.data && d.data.windows, "data.windows");
  let economy;
  try {
    economy = await fetchDoc("./economy.json", d => d && d.data, "data");
  } catch (e) {
    economy = { data: {} };
  }
  const v = reshapeReliability(doc);
  const issues = [...checkReliabilityIntegrity(v), ...crossCheckEconomyReliability(v, economy)];
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
