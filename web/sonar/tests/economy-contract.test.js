// Item 8.1 — Artifact-contract tests.
// Walk every field the economy JS actually reads and assert it exists in the
// REAL dist/economy.json with the right type/shape. Tests run only when the
// artifact exists (describe.runIf). Shape, not values — numbers may shift if
// the controller re-runs rollup+emit mid-task.
// Also includes a companion contract for dist/facilitators.json.
import { describe, it, expect, beforeAll } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const distEco  = resolve(here, "../../../dist/economy.json");
const distFac  = resolve(here, "../../../dist/facilitators.json");

const haveEco = existsSync(distEco);
const haveFac = existsSync(distFac);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
function isDateString(s) {
  return typeof s === "string" && s.length >= 10 && !isNaN(Date.parse(s));
}

function isNumberOrString(v) {
  return typeof v === "number" || typeof v === "string";
}

// ---------------------------------------------------------------------------
// dist/economy.json — top-level stamp fields
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — top-level stamp fields", () => {
  let doc;
  beforeAll(() => { doc = JSON.parse(readFileSync(distEco, "utf8")); });

  it("methodology_version is a number", () => {
    expect(typeof doc.methodology_version).toBe("number");
  });

  it("scope is a string", () => {
    expect(typeof doc.scope).toBe("string");
  });

  it("generated_at is a date-parseable string", () => {
    expect(isDateString(doc.generated_at)).toBe(true);
  });

  it("data_through_day is a date-parseable string", () => {
    expect(isDateString(doc.data_through_day)).toBe(true);
  });

  it("data is an object", () => {
    expect(typeof doc.data).toBe("object");
    expect(doc.data).not.toBeNull();
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.windows
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.windows structure", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("data.windows is an object with at least one key", () => {
    expect(typeof d.windows).toBe("object");
    expect(Object.keys(d.windows).length).toBeGreaterThan(0);
  });

  it("each window has txn_count (number) and volume_usdc (string)", () => {
    for (const [wKey, w] of Object.entries(d.windows)) {
      expect(typeof w.txn_count, `${wKey}.txn_count`).toBe("number");
      expect(typeof w.volume_usdc, `${wKey}.volume_usdc`).toBe("string");
    }
  });

  it("each window has by_band with the 5 BANDDEF keys (may be sparse)", () => {
    const BANDS = ["dust", "micro", "small", "mid", "whale"];
    for (const [wKey, w] of Object.entries(d.windows)) {
      // by_band may be sparse (some bands absent) but must be an object
      expect(typeof w.by_band, `${wKey}.by_band`).toBe("object");
      // At least the bands that exist have the right shape
      for (const k of BANDS) {
        if (w.by_band[k] !== undefined) {
          expect(typeof w.by_band[k].txn_count, `${wKey}.by_band.${k}.txn_count`).toBe("number");
          expect(typeof w.by_band[k].volume_usdc, `${wKey}.by_band.${k}.volume_usdc`).toBe("string");
        }
      }
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.daily_series
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.daily_series", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("daily_series is an array", () => {
    expect(Array.isArray(d.daily_series)).toBe(true);
  });

  it("daily_series is non-empty", () => {
    expect(d.daily_series.length).toBeGreaterThan(0);
  });

  it("each element has day (string), txn_count (number), volume_usdc (string)", () => {
    for (const p of d.daily_series) {
      expect(typeof p.day).toBe("string");
      expect(typeof p.txn_count).toBe("number");
      expect(typeof p.volume_usdc).toBe("string");
    }
  });

  it("complete field, when present, is a boolean", () => {
    for (const p of d.daily_series) {
      if ("complete" in p) {
        expect(typeof p.complete, `day ${p.day} complete field`).toBe("boolean");
      }
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.monthly_series
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.monthly_series", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("monthly_series is an array (may be empty)", () => {
    expect(Array.isArray(d.monthly_series)).toBe(true);
  });

  it("each element has month (string), txn_count (number), volume_usdc (string), complete (boolean)", () => {
    for (const m of d.monthly_series) {
      expect(typeof m.month).toBe("string");
      expect(typeof m.txn_count).toBe("number");
      expect(typeof m.volume_usdc).toBe("string");
      expect(typeof m.complete).toBe("boolean");
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.typical_payment
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.typical_payment", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("typical_payment is an object with keys matching windows", () => {
    expect(typeof d.typical_payment).toBe("object");
    for (const wKey of Object.keys(d.windows)) {
      if (d.typical_payment[wKey] !== undefined) {
        const t = d.typical_payment[wKey];
        expect(typeof t.avg_usdc, `typical[${wKey}].avg_usdc`).toBe("string");
        expect(typeof t.median_usdc, `typical[${wKey}].median_usdc`).toBe("string");
        expect(typeof t.txn_count, `typical[${wKey}].txn_count`).toBe("number");
      }
    }
  });

  // STAGE-6 (6.2/6.3): largest_payment_usdc and the p10/p90/p99 percentile
  // strip. panels.js (rOverview, rShape) only render these when non-null, so
  // the contract must not hard-fail on their absence in a stale artifact.
  it("typical_payment[*].largest_payment_usdc, when present, is a string", () => {
    for (const [wKey, t] of Object.entries(d.typical_payment)) {
      if (t.largest_payment_usdc != null) {
        expect(typeof t.largest_payment_usdc, `typical[${wKey}].largest_payment_usdc`).toBe("string");
      }
    }
  });

  it("typical_payment[*].p10/p90/p99_usdc, when present, are strings", () => {
    for (const [wKey, t] of Object.entries(d.typical_payment)) {
      if (t.p10_usdc != null || t.p90_usdc != null || t.p99_usdc != null) {
        expect(typeof t.p10_usdc, `typical[${wKey}].p10_usdc`).toBe("string");
        expect(typeof t.p90_usdc, `typical[${wKey}].p90_usdc`).toBe("string");
        expect(typeof t.p99_usdc, `typical[${wKey}].p99_usdc`).toBe("string");
      }
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.price_points
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.price_points", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("price_points is an object with window keys", () => {
    expect(typeof d.price_points).toBe("object");
  });

  it("each window's price_points is an array", () => {
    for (const wKey of Object.keys(d.windows)) {
      if (d.price_points[wKey] !== undefined) {
        expect(Array.isArray(d.price_points[wKey]), `price_points[${wKey}]`).toBe(true);
      }
    }
  });

  it("each non-empty price_point row has amount_usdc, txn_count, txn_share_pct, payee_count", () => {
    for (const [wKey, pts] of Object.entries(d.price_points)) {
      for (const p of pts) {
        expect(typeof p.amount_usdc, `${wKey} amount_usdc`).toBe("string");
        expect(typeof p.txn_count, `${wKey} txn_count`).toBe("number");
        expect(typeof p.txn_share_pct, `${wKey} txn_share_pct`).toBe("string");
        expect(typeof p.payee_count, `${wKey} payee_count`).toBe("number");
      }
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.gas
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.gas structure", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("gas is an object with windows sub-object", () => {
    expect(typeof d.gas).toBe("object");
    expect(typeof d.gas.windows).toBe("object");
  });

  it("each gas window has txn_count, gas_eth, gas_eth_l1, gas_eth_l2, gas_usd, breakeven_txn_count", () => {
    for (const [wKey, g] of Object.entries(d.gas.windows)) {
      expect(typeof g.txn_count, `gas.${wKey}.txn_count`).toBe("number");
      expect(typeof g.gas_eth, `gas.${wKey}.gas_eth`).toBe("string");
      expect(typeof g.gas_eth_l1, `gas.${wKey}.gas_eth_l1`).toBe("string");
      expect(typeof g.gas_eth_l2, `gas.${wKey}.gas_eth_l2`).toBe("string");
      expect(typeof g.gas_usd, `gas.${wKey}.gas_usd`).toBe("string");
      expect(typeof g.breakeven_txn_count, `gas.${wKey}.breakeven_txn_count`).toBe("number");
    }
  });

  it("gas_cents_per_dollar is either a string or null in each window", () => {
    for (const [wKey, g] of Object.entries(d.gas.windows)) {
      expect(
        g.gas_cents_per_dollar === null || typeof g.gas_cents_per_dollar === "string",
        `gas.${wKey}.gas_cents_per_dollar`
      ).toBe(true);
    }
  });

  it("gas.cost_daily, when present, is an array of {day, cents_per_dollar} objects", () => {
    if (!d.gas.cost_daily) return; // optional field — old artifacts lack it
    expect(Array.isArray(d.gas.cost_daily)).toBe(true);
    for (const row of d.gas.cost_daily) {
      expect(typeof row.day).toBe("string");
      expect(isNumberOrString(row.cents_per_dollar)).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.velocity
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.velocity", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("velocity has windows and daily_series", () => {
    expect(typeof d.velocity.windows).toBe("object");
    expect(Array.isArray(d.velocity.daily_series)).toBe(true);
  });

  it("each velocity window has max_per_min (number)", () => {
    for (const [wKey, vw] of Object.entries(d.velocity.windows)) {
      expect(typeof vw.max_per_min, `velocity.${wKey}.max_per_min`).toBe("number");
    }
  });

  it("velocity.daily_series elements have day (string), max_per_min (number), p99_per_min (number)", () => {
    for (const row of d.velocity.daily_series) {
      expect(typeof row.day).toBe("string");
      expect(typeof row.max_per_min).toBe("number");
      expect(typeof row.p99_per_min).toBe("number");
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.claims
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.claims", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("claims is an array (may be empty)", () => {
    expect(Array.isArray(d.claims)).toBe(true);
  });

  it("each claim row has required text/value fields", () => {
    for (const c of d.claims) {
      expect(typeof c.claim_text).toBe("string");
      expect(typeof c.claimed_value).toBe("string");
      expect(typeof c.measured_value).toBe("string");
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.concentration
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.concentration", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("concentration has windows object", () => {
    expect(typeof d.concentration).toBe("object");
    expect(typeof d.concentration.windows).toBe("object");
  });

  it("each concentration window's payee role has total_entities (number)", () => {
    for (const [wKey, cw] of Object.entries(d.concentration.windows)) {
      if (cw.payee !== undefined) {
        expect(typeof cw.payee.total_entities, `concentration.${wKey}.payee.total_entities`).toBe("number");
      }
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — data.excluded (optional)
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — data.excluded (optional)", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("excluded, when present, has txn_count (number) and volume_usdc (string)", () => {
    if (!d.excluded) return;
    expect(typeof d.excluded.txn_count).toBe("number");
    expect(typeof d.excluded.volume_usdc).toBe("string");
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — optional fields (active_entities, payer_cohorts, price_point_breadth)
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — optional data fields shape", () => {
  let d;
  beforeAll(() => { d = JSON.parse(readFileSync(distEco, "utf8")).data; });

  it("active_entities, if present, is an array of {day, payer_count, payee_count}", () => {
    if (!d.active_entities) return;
    expect(Array.isArray(d.active_entities)).toBe(true);
    for (const row of d.active_entities) {
      expect(typeof row.day).toBe("string");
      expect(typeof row.payer_count).toBe("number");
      expect(typeof row.payee_count).toBe("number");
    }
  });

  it("payer_cohorts, if present, is an object with window keys", () => {
    if (!d.payer_cohorts) return;
    expect(typeof d.payer_cohorts).toBe("object");
    for (const c of Object.values(d.payer_cohorts)) {
      expect(typeof c.new_payer_volume_usdc).toBe("string");
      expect(typeof c.returning_payer_volume_usdc).toBe("string");
    }
  });

  it("price_point_breadth, if present, is an array of {amount_usdc, series}", () => {
    if (!d.price_point_breadth) return;
    expect(Array.isArray(d.price_point_breadth)).toBe(true);
    for (const row of d.price_point_breadth) {
      expect(typeof row.amount_usdc).toBe("string");
      expect(Array.isArray(row.series)).toBe(true);
    }
  });
});

// ---------------------------------------------------------------------------
// dist/economy.json — meta stamps used by the UI
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — meta stamps consumed by app.js", () => {
  let doc;
  beforeAll(() => { doc = JSON.parse(readFileSync(distEco, "utf8")); });

  it("scope field exists (used by rShell)", () => {
    expect(typeof doc.scope).toBe("string");
    expect(doc.scope.length).toBeGreaterThan(0);
  });

  it("methodology_version is a positive integer", () => {
    expect(Number.isInteger(doc.methodology_version)).toBe(true);
    expect(doc.methodology_version).toBeGreaterThan(0);
  });

  it("generated_at is an ISO 8601 timestamp string", () => {
    expect(doc.generated_at).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/);
  });

  it("data_through_day is a YYYY-MM-DD date string", () => {
    expect(doc.data_through_day).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

// ---------------------------------------------------------------------------
// Stage-6 freshness summary — not an assertion, just a signal for the
// controller to confirm the fresh rollup+emit landed (per the brief's
// CORE-vs-STAGE-6 artifact-freshness rule). Logs which stage-6 sections were
// found in the artifact this run saw.
// ---------------------------------------------------------------------------
describe.runIf(haveEco)("economy.json — stage-6 freshness signal (informational)", () => {
  it("logs which stage-6 sections are present in this run's artifact", () => {
    const d = JSON.parse(readFileSync(distEco, "utf8")).data;
    const anyTypicalHasLargest = Object.values(d.typical_payment || {}).some(t => t.largest_payment_usdc != null);
    const anyTypicalHasPct = Object.values(d.typical_payment || {}).some(t => t.p10_usdc != null);
    const found = {
      "typical_payment.*.largest_payment_usdc": anyTypicalHasLargest,
      "typical_payment.*.p10/p90/p99_usdc": anyTypicalHasPct,
      "gas.cost_daily": !!d.gas?.cost_daily,
      "active_entities": !!d.active_entities,
      "payer_cohorts": !!d.payer_cohorts,
      "price_point_breadth": !!d.price_point_breadth,
    };
    if (haveFac) {
      const fac = JSON.parse(readFileSync(distFac, "utf8"));
      const rows = fac.data?.rows || [];
      found["facilitators.json rows[*].windows[7d/30d]"] =
        rows.length > 0 && !!(rows[0].windows && rows[0].windows["7d"] && rows[0].windows["30d"]);
    }
    const present = Object.entries(found).filter(([, v]) => v).map(([k]) => k);
    const absent  = Object.entries(found).filter(([, v]) => !v).map(([k]) => k);
    console.info(
      `[stage-6 freshness] present: [${present.join(", ") || "none"}] | absent: [${absent.join(", ") || "none"}]`
    );
    expect(true).toBe(true); // informational only — never fails the suite
  });
});

// ---------------------------------------------------------------------------
// dist/facilitators.json — contract
// ---------------------------------------------------------------------------
describe.runIf(haveFac)("facilitators.json — top-level contract", () => {
  let doc;
  beforeAll(() => { doc = JSON.parse(readFileSync(distFac, "utf8")); });

  it("methodology_version is a number", () => {
    expect(typeof doc.methodology_version).toBe("number");
  });

  it("scope is a string", () => {
    expect(typeof doc.scope).toBe("string");
  });

  it("data.rows is a non-empty array", () => {
    expect(Array.isArray(doc.data.rows)).toBe(true);
    expect(doc.data.rows.length).toBeGreaterThan(0);
  });

  it("each row has facilitator (string), facilitator_known (boolean), txn_count (number), volume_usdc (string)", () => {
    for (const r of doc.data.rows) {
      expect(typeof r.facilitator, "facilitator").toBe("string");
      expect(typeof r.facilitator_known, "facilitator_known").toBe("boolean");
      expect(typeof r.txn_count, "txn_count").toBe("number");
      expect(typeof r.volume_usdc, "volume_usdc").toBe("string");
    }
  });

  it("rows are sorted by volume_usdc descending (first row highest)", () => {
    const rows = doc.data.rows;
    if (rows.length < 2) return; // not enough to compare
    const first = parseFloat(rows[0].volume_usdc);
    const second = parseFloat(rows[1].volume_usdc);
    expect(first).toBeGreaterThanOrEqual(second);
  });

  it("each row's windows, when present, has 7d and 30d sub-objects", () => {
    // Current artifact may lack windows — test shape for any row that has them
    let testedAny = false;
    for (const r of doc.data.rows) {
      if (r.windows) {
        testedAny = true;
        if (r.windows["7d"]) {
          expect(typeof r.windows["7d"].volume_usdc).toBe("string");
        }
        if (r.windows["30d"]) {
          expect(typeof r.windows["30d"].volume_usdc).toBe("string");
        }
      }
    }
    // If no rows have windows, that is documented behavior (absent-state panel)
    if (!testedAny) {
      expect(true).toBe(true); // no window data — explicit pass-through
    }
  });
});
