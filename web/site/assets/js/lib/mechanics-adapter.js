// Loads mechanics.json, validates the envelope, and re-checks the invariants the
// emitter promised: known+unknown==settlement, batch payment_count sums to
// settlement, pct_batched in [0,1], settlement_count==economy txn_count. The
// selector mix is top-15 and tx_type buckets only 0/1/2 — those are surfaced as
// remainders, NEVER asserted to conserve. Mirrors reliability-adapter.js.

// Known x402 method selectors → friendly names. Unknowns show raw hex.
export const SELECTOR_LABELS = {
  e3ee160e: "transferWithAuthorization",
  cf092995: "transferWithAuthorization",
  "82ad56cb": "Multicall3.aggregate3",
};

export function selectorLabel(hex) {
  return SELECTOR_LABELS[hex] || null;
}

export function reshapeMechanics(doc) {
  const d = doc.data;
  return {
    meta: {
      methodology_version: doc.methodology_version,
      generated_at: doc.generated_at,
      data_through_day: doc.data_through_day,
    },
    windows: d.windows,
  };
}

// txTypeOther = settlement_count - (type0 + type1 + type2); the OP-stack remainder.
export function txTypeOther(measure) {
  const t = measure.fee.tx_type;
  return measure.settlement_count - ((t["0"] || 0) + (t["1"] || 0) + (t["2"] || 0));
}

export function checkMechanicsIntegrity(view) {
  const issues = [];
  if (view.meta.methodology_version !== 1) {
    issues.push({ level: "warn", msg: `methodology v${view.meta.methodology_version} — this page was built for v1; review before citing` });
  }
  for (const [w, win] of Object.entries(view.windows)) {
    const k = win.by_membership && win.by_membership.known;
    const u = win.by_membership && win.by_membership.unknown;
    if (k && u && (k.settlement_count + u.settlement_count) !== win.settlement_count) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} known+unknown ${k.settlement_count + u.settlement_count} ≠ all ${win.settlement_count}` });
    }
    const batchSum = (win.batch.histogram || []).reduce((s, b) => s + b.payment_count, 0);
    if (batchSum !== win.settlement_count) {
      issues.push({ level: "error", msg: `conservation ✗ — ${w} batch payments ${batchSum} ≠ settlements ${win.settlement_count}` });
    }
    const p = win.batch.pct_batched;
    if (typeof p === "number" && (p < 0 || p > 1)) {
      issues.push({ level: "error", msg: `range ✗ — ${w} pct_batched ${p} outside [0,1]` });
    }
    if (txTypeOther(win) < 0) {
      issues.push({ level: "error", msg: `range ✗ — ${w} tx_type buckets exceed settlements` });
    }
  }
  return issues;
}

export function crossCheckEconomyMechanics(view, economyDoc) {
  const ec = economyDoc && economyDoc.data && economyDoc.data.windows;
  if (!ec) return [{ level: "warn", msg: "economy.json has no windows to cross-check" }];
  const all = view.windows.all, eall = ec.all;
  if (!all || !eall) return [{ level: "warn", msg: "no all-window to cross-check against economy" }];
  if (all.settlement_count !== eall.txn_count) {
    return [{ level: "error", msg: `cross-check ✗ — settlements ${all.settlement_count} ≠ economy txns ${eall.txn_count}` }];
  }
  return [];
}

export async function loadMechanics() {
  const doc = await fetchDoc("./mechanics.json", d => d && d.data && d.data.windows, "data.windows");
  let economy;
  try {
    economy = await fetchDoc("./economy.json", d => d && d.data, "data");
  } catch (e) {
    economy = { data: {} };
  }
  const v = reshapeMechanics(doc);
  const issues = [...checkMechanicsIntegrity(v), ...crossCheckEconomyMechanics(v, economy)];
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
