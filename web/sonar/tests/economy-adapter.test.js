// Item 8.2 — adapter unit tests.
// Covers: reshape densification (missing band gets zero-filled), checkIntegrity
// full pass/fail matrix ({name,pass,detail,level,msg} contract), winLabels,
// ZERO_GAS fill, REQUIRED_SECTIONS error naming ALL missing sections.
import { describe, it, expect } from "vitest";
import { reshape, checkIntegrity, winLabels, USD_TOLERANCE } from "../app/assets/js/adapter.js";

// ---------------------------------------------------------------------------
// Minimal valid artifact helper
// ---------------------------------------------------------------------------
function makeDoc(overrides = {}) {
  return {
    methodology_version: 1,
    scope: "verified-x402",
    generated_at: "2026-06-18T00:00:00Z",
    data_through_day: "2026-06-18",
    data: {
      windows: {
        all: {
          txn_count: 100,
          volume_usdc: "50.00",
          by_band: {
            dust:  { txn_count: 20, volume_usdc: "0.20" },
            micro: { txn_count: 20, volume_usdc: "2.00" },
            small: { txn_count: 20, volume_usdc: "20.00" },
            mid:   { txn_count: 20, volume_usdc: "10.00" },
            whale: { txn_count: 20, volume_usdc: "17.80" },
          },
        },
        "7d": {
          txn_count: 10,
          volume_usdc: "5.00",
          by_band: {
            dust:  { txn_count: 2, volume_usdc: "0.02" },
            micro: { txn_count: 2, volume_usdc: "0.20" },
            small: { txn_count: 2, volume_usdc: "2.00" },
            mid:   { txn_count: 2, volume_usdc: "1.00" },
            whale: { txn_count: 2, volume_usdc: "1.78" },
          },
        },
      },
      daily_series: [{ day: "2026-06-18", txn_count: 100, volume_usdc: "50.00" }],
      monthly_series: [{ month: "2026-06", complete: true, txn_count: 100, volume_usdc: "50.00" }],
      typical_payment: {
        all: { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 100 },
        "7d": { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 10 },
      },
      price_points: { all: [], "7d": [] },
      gas: {
        windows: {
          all: {
            txn_count: 100, gas_eth: "0.01", gas_eth_l1: "0.005", gas_eth_l2: "0.005",
            gas_usd: "25.00", gas_cents_per_dollar: "0.50", breakeven_txn_count: 50,
            by_band: {},
          },
          "7d": {
            txn_count: 10, gas_eth: "0.001", gas_eth_l1: "0.0005", gas_eth_l2: "0.0005",
            gas_usd: "2.50", gas_cents_per_dollar: "0.50", breakeven_txn_count: 5,
            by_band: {},
          },
        },
      },
      velocity: {
        windows: {
          all: { max_per_min: 100 },
          "7d": { max_per_min: 80 },
        },
        daily_series: [{ day: "2026-06-18", max_per_min: 100, p99_per_min: 90 }],
      },
      claims: [],
      concentration: { windows: { all: { payee: { total_entities: 50 } } } },
      excluded: { txn_count: 500, volume_usdc: "100.00" },
      ...overrides,
    },
  };
}

// ---------------------------------------------------------------------------
// REQUIRED_SECTIONS — names ALL missing sections in one error
// ---------------------------------------------------------------------------
describe("reshape — REQUIRED_SECTIONS error", () => {
  it("throws when a required section is missing, naming it", () => {
    const doc = makeDoc();
    delete doc.data.daily_series;
    expect(() => reshape(doc)).toThrow(/daily_series/);
  });

  it("throws when gas section is missing", () => {
    const doc = makeDoc();
    delete doc.data.gas;
    expect(() => reshape(doc)).toThrow(/gas/);
  });

  it("throws when velocity section is missing", () => {
    const doc = makeDoc();
    delete doc.data.velocity;
    expect(() => reshape(doc)).toThrow(/velocity/);
  });

  it("names ALL missing sections in one error, not just the first", () => {
    const doc = makeDoc();
    delete doc.data.daily_series;
    delete doc.data.monthly_series;
    delete doc.data.velocity;
    let msg = "";
    try { reshape(doc); } catch (e) { msg = e.message; }
    expect(msg).toContain("daily_series");
    expect(msg).toContain("monthly_series");
    expect(msg).toContain("velocity");
  });

  it("does NOT throw for optional sections (claims, concentration, excluded absent)", () => {
    const doc = makeDoc();
    delete doc.data.claims;
    delete doc.data.concentration;
    delete doc.data.excluded;
    expect(() => reshape(doc)).not.toThrow();
  });

  it("complete valid artifact does not throw", () => {
    expect(() => reshape(makeDoc())).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// reshape densification — sparse bands get zero-filled
// ---------------------------------------------------------------------------
describe("reshape densification — missing band gets zero-filled", () => {
  it("7d window missing 'whale' band gets {txn_count:0, volume_usdc:'0'}", () => {
    const doc = makeDoc();
    // Remove whale from 7d — simulates the live missing-whale-band case
    delete doc.data.windows["7d"].by_band.whale;
    const view = reshape(doc);
    const whale7 = view.windows["7d"].by_band.whale;
    expect(whale7).toBeDefined();
    expect(whale7.txn_count).toBe(0);
    expect(whale7.volume_usdc).toBe("0");
  });

  it("all 5 bands are always present after densification", () => {
    const doc = makeDoc();
    // Give 7d window only dust band
    doc.data.windows["7d"].by_band = { dust: { txn_count: 10, volume_usdc: "0.01" } };
    const view = reshape(doc);
    const bandKeys = ["dust", "micro", "small", "mid", "whale"];
    for (const k of bandKeys) {
      expect(view.windows["7d"].by_band).toHaveProperty(k);
    }
  });

  it("existing bands are not replaced by zero", () => {
    const doc = makeDoc();
    const view = reshape(doc);
    expect(view.windows.all.by_band.dust.txn_count).toBe(20);
    expect(view.windows.all.by_band.dust.volume_usdc).toBe("0.20");
  });
});

// ---------------------------------------------------------------------------
// reshape — ZERO_GAS fill for missing gas windows
// ---------------------------------------------------------------------------
describe("reshape — ZERO_GAS fill for absent gas windows", () => {
  it("gas window absent → filled with ZERO_GAS fields", () => {
    const doc = makeDoc();
    // Remove gas for "7d" window entirely
    delete doc.data.gas.windows["7d"];
    const view = reshape(doc);
    const g7 = view.gas.windows["7d"];
    expect(g7).toBeDefined();
    expect(g7.txn_count).toBe(0);
    expect(g7.gas_eth).toBe("0");
    expect(g7.gas_eth_l1).toBe("0");
    expect(g7.gas_eth_l2).toBe("0");
    expect(g7.gas_usd).toBe("0");
    expect(g7.breakeven_txn_count).toBe(0);
    // gas_cents_per_dollar is null in ZERO_GAS
    expect(g7.gas_cents_per_dollar).toBeNull();
  });

  it("gas window present → original fields preserved", () => {
    const view = reshape(makeDoc());
    expect(view.gas.windows.all.gas_eth).toBe("0.01");
    expect(view.gas.windows.all.breakeven_txn_count).toBe(50);
  });

  it("gas.cost_daily absent → empty array in view", () => {
    const view = reshape(makeDoc());
    // makeDoc has no cost_daily → adapter produces []
    expect(Array.isArray(view.gas.cost_daily)).toBe(true);
    expect(view.gas.cost_daily).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// reshape — daily series compact tuple [day, txn_count, volume_cents, complete]
// ---------------------------------------------------------------------------
describe("reshape — daily series compact tuple", () => {
  it("produces [day, txn_count, rounded-volume, complete] 4-tuple", () => {
    const view = reshape(makeDoc());
    const [day, count, vol, complete] = view.daily[0];
    expect(typeof day).toBe("string");
    expect(typeof count).toBe("number");
    expect(typeof vol).toBe("number");
    expect(typeof complete).toBe("boolean");
  });

  it("complete defaults to true when field absent (old-artifact tolerance)", () => {
    const doc = makeDoc();
    doc.data.daily_series = [{ day: "2026-06-18", txn_count: 100, volume_usdc: "50.00" }];
    const view = reshape(doc);
    expect(view.daily[0][3]).toBe(true);
  });

  it("complete === false is preserved when explicitly false", () => {
    const doc = makeDoc();
    doc.data.daily_series = [{ day: "2026-06-18", txn_count: 100, volume_usdc: "50.00", complete: false }];
    const view = reshape(doc);
    expect(view.daily[0][3]).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// winLabels — derive "all" label from monthly data
// ---------------------------------------------------------------------------
describe("winLabels", () => {
  it("returns correct shape with 7d, 30d, all keys", () => {
    const view = reshape(makeDoc());
    const labels = winLabels(view);
    expect(labels).toHaveProperty("7d");
    expect(labels).toHaveProperty("30d");
    expect(labels).toHaveProperty("all");
    expect(typeof labels["7d"]).toBe("string");
    expect(typeof labels["30d"]).toBe("string");
    expect(typeof labels.all).toBe("string");
  });

  it("7d label is 'trailing 7d'", () => {
    const labels = winLabels(reshape(makeDoc()));
    expect(labels["7d"]).toBe("trailing 7d");
  });

  it("30d label is 'trailing 30d'", () => {
    const labels = winLabels(reshape(makeDoc()));
    expect(labels["30d"]).toBe("trailing 30d");
  });

  it("all label includes month names derived from monthly series", () => {
    const doc = makeDoc();
    doc.data.monthly_series = [
      { month: "2026-01", complete: true, txn_count: 50, volume_usdc: "25.00" },
      { month: "2026-06", complete: true, txn_count: 100, volume_usdc: "50.00" },
    ];
    const labels = winLabels(reshape(doc));
    expect(labels.all).toContain("Jan");
    expect(labels.all).toContain("Jun");
    expect(labels.all).toContain("all data");
  });

  it("all label is 'all data' when monthly series is empty", () => {
    const doc = makeDoc();
    doc.data.monthly_series = [];
    const labels = winLabels(reshape(doc));
    expect(labels.all).toBe("all data");
  });
});

// ---------------------------------------------------------------------------
// checkIntegrity — {name, pass, detail, level, msg} contract
// ---------------------------------------------------------------------------
describe("checkIntegrity — return type contract", () => {
  it("returns an array of check objects", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    expect(Array.isArray(checks)).toBe(true);
    expect(checks.length).toBeGreaterThan(0);
  });

  it("each check has required fields: name, pass, detail, level, msg", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    for (const c of checks) {
      expect(c).toHaveProperty("name");
      expect(c).toHaveProperty("pass");
      expect(c).toHaveProperty("detail");
      expect(c).toHaveProperty("level");
      expect(c).toHaveProperty("msg");
      expect(typeof c.name).toBe("string");
      expect(typeof c.pass).toBe("boolean");
      expect(typeof c.detail).toBe("string");
    }
  });

  it("passing check has level='ok' and msg=null", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const passing = checks.filter(c => c.pass);
    expect(passing.length).toBeGreaterThan(0);
    for (const c of passing) {
      expect(c.level).toBe("ok");
      expect(c.msg).toBeNull();
    }
  });
});

describe("checkIntegrity — methodology check", () => {
  it("methodology v1 passes", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const methCheck = checks.find(c => c.name === "methodology");
    expect(methCheck.pass).toBe(true);
    expect(methCheck.level).toBe("ok");
  });

  it("methodology v2 warns (not error)", () => {
    const view = reshape(makeDoc());
    view.meta.methodology_version = 2;
    const checks = checkIntegrity(view);
    const methCheck = checks.find(c => c.name === "methodology");
    expect(methCheck.pass).toBe(false);
    expect(methCheck.level).toBe("warn");
    expect(typeof methCheck.msg).toBe("string");
    expect(methCheck.msg).not.toBe("");
  });
});

describe("checkIntegrity — conservation checks", () => {
  it("passing: txn counts conserve exactly across bands", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const countCheck = checks.find(c => c.name === "all-count");
    expect(countCheck.pass).toBe(true);
  });

  it("FAILING: txn count mismatch → pass=false, level='error', non-null msg", () => {
    const view = reshape(makeDoc());
    // Break band sum: add extra count that doesn't match window total
    view.windows.all.txn_count = 999; // real band sum is 100
    const checks = checkIntegrity(view);
    const countCheck = checks.find(c => c.name === "all-count");
    expect(countCheck.pass).toBe(false);
    expect(countCheck.level).toBe("error");
    expect(typeof countCheck.msg).toBe("string");
    expect(countCheck.msg.length).toBeGreaterThan(0);
  });

  it("passing: USD sums conserve within $0.50 tolerance", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const usdCheck = checks.find(c => c.name === "all-usd");
    expect(usdCheck.pass).toBe(true);
  });

  it("FAILING: USD off by more than $0.50 → pass=false, level='error'", () => {
    const view = reshape(makeDoc());
    // Set window total to something far off the band sum
    view.windows.all.volume_usdc = "100.00"; // band sum is ~50
    const checks = checkIntegrity(view);
    const usdCheck = checks.find(c => c.name === "all-usd");
    expect(usdCheck.pass).toBe(false);
    expect(usdCheck.level).toBe("error");
  });

  it("USD_TOLERANCE boundary: exactly $0.50 off → PASSES", () => {
    const view = reshape(makeDoc());
    // band sum = 20+2+10+0.20+17.80 = 50.00, window = 50.00 — add exactly 0.50
    view.windows.all.volume_usdc = "50.50";
    const checks = checkIntegrity(view);
    const usdCheck = checks.find(c => c.name === "all-usd");
    expect(usdCheck.pass).toBe(true);
  });

  it("USD_TOLERANCE boundary: $0.51 off → FAILS", () => {
    const view = reshape(makeDoc());
    view.windows.all.volume_usdc = "50.51";
    const checks = checkIntegrity(view);
    const usdCheck = checks.find(c => c.name === "all-usd");
    expect(usdCheck.pass).toBe(false);
  });

  it("produces check entries for every window in the view", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const winKeys = Object.keys(view.windows);
    for (const w of winKeys) {
      expect(checks.some(c => c.name === `${w}-count`)).toBe(true);
      expect(checks.some(c => c.name === `${w}-usd`)).toBe(true);
    }
  });

  it("USD_TOLERANCE exported constant is 0.5", () => {
    expect(USD_TOLERANCE).toBe(0.5);
  });
});

describe("checkIntegrity — excluded-remainder check", () => {
  it("excluded-remainder check appears when view.excluded is present", () => {
    const view = reshape(makeDoc());
    const checks = checkIntegrity(view);
    const exclCheck = checks.find(c => c.name === "excluded-remainder");
    expect(exclCheck).toBeDefined();
    expect(exclCheck.pass).toBe(true);
    expect(exclCheck.level).toBe("ok");
  });

  it("no excluded-remainder check when excluded is absent", () => {
    const doc = makeDoc();
    delete doc.data.excluded;
    const view = reshape(doc);
    const checks = checkIntegrity(view);
    expect(checks.find(c => c.name === "excluded-remainder")).toBeUndefined();
  });
});
