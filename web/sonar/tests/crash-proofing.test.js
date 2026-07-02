// Tests for crash-proofing items 4.1 - 4.4.
// Item 4.5 (loading state) is in loading.test.js because it requires a fresh app.js import.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel, setIssues } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { rGas, rClaims } from "../app/assets/js/panels.js";
import { rDaily } from "../app/assets/js/charts.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { claimVerdict } from "../app/assets/js/format.js";

// ---------------------------------------------------------------------------
// Minimal raw-artifact doc builder for reshape tests
// ---------------------------------------------------------------------------
function makeDoc(dataOverrides = {}) {
  return {
    methodology_version: 1,
    generated_at: "2026-06-18T00:00:00Z",
    data_through_day: "2026-06-18",
    data: {
      windows: {
        all:  { txn_count: 200, volume_usdc: "100.00", by_band: {
          dust:  { txn_count: 40, volume_usdc: "0.40" },
          micro: { txn_count: 40, volume_usdc: "4.00" },
          small: { txn_count: 40, volume_usdc: "40.00" },
          mid:   { txn_count: 40, volume_usdc: "20.00" },
          whale: { txn_count: 40, volume_usdc: "35.60" },
        }},
        "7d": { txn_count: 50, volume_usdc: "25.00", by_band: {
          dust:  { txn_count: 10, volume_usdc: "0.10" },
          micro: { txn_count: 10, volume_usdc: "1.00" },
          small: { txn_count: 10, volume_usdc: "10.00" },
          mid:   { txn_count: 10, volume_usdc: "5.00" },
          whale: { txn_count: 10, volume_usdc: "8.90" },
        }},
        "30d":{ txn_count: 150, volume_usdc: "75.00", by_band: {
          dust:  { txn_count: 30, volume_usdc: "0.30" },
          micro: { txn_count: 30, volume_usdc: "3.00" },
          small: { txn_count: 30, volume_usdc: "30.00" },
          mid:   { txn_count: 30, volume_usdc: "15.00" },
          whale: { txn_count: 30, volume_usdc: "26.70" },
        }},
      },
      daily_series: [{ day: "2026-06-18", txn_count: 100, volume_usdc: "50.00" }],
      monthly_series: [],
      typical_payment: {
        all:  { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 200 },
        "7d": { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 50 },
        "30d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 150 },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: {
        windows: {
          all: {
            txn_count: 200, breakeven_txn_count: 100,
            gas_eth: "0.010", gas_eth_l1: "0.003", gas_eth_l2: "0.007",
            gas_usd: "25.00", gas_cents_per_dollar: "25.00",
          },
          // 7d and 30d windows intentionally absent to test 4.1
        },
      },
      velocity: {
        windows: { all: { max_per_min: 10 }, "7d": { max_per_min: 5 }, "30d": { max_per_min: 8 } },
        daily_series: [],
      },
      ...dataOverrides,
    },
  };
}

// ---------------------------------------------------------------------------
// Shared view (post-reshape) for panel/tray render tests
// ---------------------------------------------------------------------------
const baseView = {
  meta: { methodology_version: 1, data_through_day: "2026-06-18", generated_at: "2026-06-18T00:00:00Z" },
  windows: {
    all:  { txn_count: 200, volume_usdc: "100.00", by_band: {
      dust:  { txn_count: 40, volume_usdc: "0.40" },
      micro: { txn_count: 40, volume_usdc: "4.00" },
      small: { txn_count: 40, volume_usdc: "40.00" },
      mid:   { txn_count: 40, volume_usdc: "20.00" },
      whale: { txn_count: 40, volume_usdc: "35.60" },
    }},
    "7d": { txn_count: 50, volume_usdc: "25.00", by_band: {
      dust:  { txn_count: 10, volume_usdc: "0.10" },
      micro: { txn_count: 10, volume_usdc: "1.00" },
      small: { txn_count: 10, volume_usdc: "10.00" },
      mid:   { txn_count: 10, volume_usdc: "5.00" },
      whale: { txn_count: 10, volume_usdc: "8.90" },
    }},
    "30d":{ txn_count: 150, volume_usdc: "75.00", by_band: {
      dust:  { txn_count: 30, volume_usdc: "0.30" },
      micro: { txn_count: 30, volume_usdc: "3.00" },
      small: { txn_count: 30, volume_usdc: "30.00" },
      mid:   { txn_count: 30, volume_usdc: "15.00" },
      whale: { txn_count: 30, volume_usdc: "26.70" },
    }},
  },
  daily: [["2026-06-18", 100, 50.0]],
  monthly: [],
  typical: {
    all:  { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 200 },
    "7d": { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 50 },
    "30d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 150 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 200, breakeven_txn_count: 100, gas_eth: "0.010", gas_eth_l1: "0.003", gas_eth_l2: "0.007", gas_usd: "25.00", gas_cents_per_dollar: "25.00", by_band: {} },
    "7d": { txn_count: 0, breakeven_txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null, by_band: {} },
    "30d":{ txn_count: 0, breakeven_txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null, by_band: {} },
  }},
  velocity: {
    windows: { all: { max_per_min: 10 }, "7d": { max_per_min: 5 }, "30d": { max_per_min: 8 } },
    verified_daily: [],
  },
  claims: [],
  concentration: { windows: {} },
  excluded: null,
};

function gasDom() {
  document.body.innerHTML = `
    <span id="gas-win"></span>
    <span id="gas-pct"></span>
    <div id="gas-meter"></div>
    <div id="gas-kv"></div>
    <table id="gasbands"></table>`;
}

function dailyDom() {
  document.body.innerHTML = `
    <div id="dailychart" style="width:900px"></div>
    <span id="d-range"></span>
    <span id="d-readout"></span>`;
}

function claimsDom() {
  document.body.innerHTML = `<div id="claims"></div>`;
}

// ===========================================================================
// Item 4.1 — missing gas window must not kill the render
// ===========================================================================
describe("4.1 - missing gas window renders with zeros", () => {
  it("reshape: missing 7d gas window produces ZERO_GAS values (not undefined)", () => {
    const doc = makeDoc();  // only has gas.windows.all; 7d and 30d absent
    const view = reshape(doc);
    // gas_eth must be "0" (from ZERO_GAS), not undefined
    expect(view.gas.windows["7d"].gas_eth).toBe("0");
    expect(view.gas.windows["7d"].gas_eth_l1).toBe("0");
    expect(view.gas.windows["7d"].gas_eth_l2).toBe("0");
    expect(view.gas.windows["7d"].gas_usd).toBe("0");
    expect(view.gas.windows["7d"].txn_count).toBe(0);
    expect(view.gas.windows["7d"].breakeven_txn_count).toBe(0);
    expect(view.gas.windows["7d"].gas_cents_per_dollar).toBe(null);
  });

  it("reshape: present gas window is NOT overwritten by ZERO_GAS", () => {
    const doc = makeDoc();
    const view = reshape(doc);
    expect(view.gas.windows.all.gas_eth).toBe("0.010");
    expect(view.gas.windows.all.txn_count).toBe(200);
  });

  it("rGas: renders without throwing when 7d gas window is all zeros", () => {
    gasDom();
    setData(baseView);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "7d";
    // Must not throw; gas_eth "0" -> num("0") = 0 -> 0..toFixed(3) = "0.000"
    expect(() => rGas()).not.toThrow();
    expect(document.getElementById("gas-kv").innerHTML).toContain("0.000");
  });

  it("rGas: breakeven shows '—' when txn_count is 0", () => {
    gasDom();
    setData(baseView);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "7d";
    rGas();
    // 100 * 0 / 0 = NaN => isFinite(NaN) = false => "—"
    expect(document.getElementById("gas-pct").textContent).toBe("—");
  });
});

// ===========================================================================
// Item 4.2 — empty daily series must render an empty state
// ===========================================================================
describe("4.2 - empty daily series renders an empty state", () => {
  beforeEach(() => {
    dailyDom();
    const emptyView = { ...baseView, daily: [] };
    setData(emptyView);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.dWin = "all";
    state.dMetric = "tx";
    state.dScale = "lin";
    state.dMa = "raw";
    if (_clearPins) _clearPins();
  });

  it("rDaily: does not throw on empty daily series", () => {
    expect(() => rDaily()).not.toThrow();
  });

  it("rDaily: renders a visible empty-state message (no data)", () => {
    rDaily();
    const html = document.getElementById("dailychart").innerHTML;
    expect(html.toLowerCase()).toMatch(/no.*(data|verified|payment)/);
  });

  it("rDaily: does NOT crash on days[0][0] (would throw TypeError on empty array)", () => {
    // Before the fix, accessing days[0][0] when days=[] throws TypeError
    // After the fix, we return early before that line
    let threw = false;
    try { rDaily(); } catch (e) { threw = true; }
    expect(threw).toBe(false);
  });

  it("PINNERS.daily: returns null when the slice is empty", () => {
    state.dWin = "all";
    const pin = PINNERS.daily();
    // slice = [] → should return null, not throw on .reduce
    expect(pin).toBeNull();
  });

  it("rDaily: all-zero values in log mode produce no NaN in SVG output", () => {
    // Set up a view with non-empty but all-zero values in log mode
    const zeroView = {
      ...baseView,
      daily: [
        ["2026-06-15", 0, 0],
        ["2026-06-16", 0, 0],
        ["2026-06-17", 0, 0],
      ],
    };
    setData(zeroView);
    state.dScale = "log";
    state.dMetric = "usd";
    state.dWin = "all";
    expect(() => rDaily()).not.toThrow();
    const svgHtml = document.getElementById("dailychart").innerHTML;
    // Should not contain NaN coordinate values in SVG path data
    expect(svgHtml).not.toMatch(/[Md]NaN/);
  });
});

// ===========================================================================
// Item 4.3 — claims math guards
// ===========================================================================
describe("4.3 - claims: zero measured_value renders not-comparable", () => {
  beforeEach(() => {
    claimsDom();
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("claimVerdict: returns 'not comparable' for null ratio", () => {
    expect(claimVerdict(null)).toBe("not comparable");
  });

  it("claimVerdict: returns 'not comparable' for Infinity (zero measured)", () => {
    expect(claimVerdict(Infinity)).toBe("not comparable");
  });

  it("claimVerdict: returns 'not comparable' for NaN", () => {
    expect(claimVerdict(NaN)).toBe("not comparable");
  });

  it("claimVerdict: still works correctly for normal ratios", () => {
    expect(claimVerdict(2.0)).toContain("higher than measured");
    expect(claimVerdict(0.5)).toBe("lower than measured");
    expect(claimVerdict(1.0)).toBe("matches measurement");
  });

  it("rClaims: zero measured_value shows 'not comparable' text in panel", () => {
    const view = {
      ...baseView,
      claims: [{
        claim_text: "x402 processes $10M monthly",
        source: "SomeSource",
        source_url: "https://example.com",
        claim_date: "2026-01-01",
        claimed_value: "10000000",
        measured_value: "0",
        measured_unit: "USDC",
        measured_metric: "total volume",
      }],
    };
    setData(view);
    rClaims();
    const html = document.getElementById("claims").innerHTML;
    expect(html).toContain("not comparable");
  });

  it("rClaims: does not show 'Infinity' or 'NaN' in verdict when measured is 0", () => {
    const view = {
      ...baseView,
      claims: [{
        claim_text: "zero-measured claim",
        source: "S",
        source_url: "https://example.com",
        claim_date: "2026-01-01",
        claimed_value: "5000",
        measured_value: "0",
        measured_unit: "USDC",
        measured_metric: "volume",
      }],
    };
    setData(view);
    rClaims();
    const html = document.getElementById("claims").innerHTML;
    expect(html).not.toContain("Infinity");
    expect(html).not.toContain("NaN");
  });

  it("rClaims: claim_text containing HTML tags renders inert (escaped)", () => {
    const view = {
      ...baseView,
      claims: [{
        claim_text: '<b>inject</b> and "quotes"',
        source: "S",
        source_url: "https://example.com",
        claim_date: "2026-01-01",
        claimed_value: "100",
        measured_value: "50",
        measured_unit: "USDC",
        measured_metric: "volume",
      }],
    };
    setData(view);
    rClaims();
    const el = document.getElementById("claims");
    // The text should contain the literal <b> characters, not a rendered <b> element
    expect(el.querySelector(".q")?.innerHTML).toContain("&lt;b&gt;inject&lt;/b&gt;");
    // No live <b> child element should be present in the .q div
    expect(el.querySelector(".q b")).toBeNull();
  });

  it("PINNERS.claims: returns null when measured_value is 0 (rather than crashing or showing Infinity)", () => {
    const view = {
      ...baseView,
      claims: [{
        claim_text: "zero measured",
        source: "S",
        source_url: "https://example.com",
        claim_date: "2026-01-01",
        claimed_value: "999",
        measured_value: "0",
        measured_unit: "USDC",
        measured_metric: "volume",
      }],
    };
    setData(view);
    // Should not throw; verdict should be 'not comparable', not 'Infinity× higher...'
    let result;
    expect(() => { result = PINNERS.claims(); }).not.toThrow();
    expect(result?.value).not.toContain("Infinity");
    expect(result?.value).toContain("not comparable");
  });
});

// ===========================================================================
// Item 4.4 — named shape errors instead of undefined-read crashes
// ===========================================================================
describe("4.4 - named shape errors for missing artifact sections", () => {
  it("reshape: throws when gas section is missing, naming it in the error", () => {
    const doc = makeDoc({ gas: undefined });
    expect(() => reshape(doc)).toThrow(/artifact missing sections:.*gas/i);
  });

  it("reshape: throws when velocity section is missing, naming it in the error", () => {
    const doc = makeDoc({ velocity: undefined });
    expect(() => reshape(doc)).toThrow(/artifact missing sections:.*velocity/i);
  });

  it("reshape: names ALL missing sections in one error (not just the first)", () => {
    const doc = makeDoc({ gas: undefined, velocity: undefined });
    let err;
    try { reshape(doc); } catch (e) { err = e; }
    expect(err).toBeDefined();
    expect(err.message).toMatch(/gas/i);
    expect(err.message).toMatch(/velocity/i);
  });

  it("reshape: throws when daily_series is missing", () => {
    const doc = makeDoc({ daily_series: undefined });
    expect(() => reshape(doc)).toThrow(/artifact missing sections:.*daily_series/i);
  });

  it("reshape: complete artifact does NOT throw", () => {
    const doc = makeDoc();  // has all required sections
    expect(() => reshape(doc)).not.toThrow();
  });

  it("reshape: optional sections (claims, concentration, excluded) absent does not throw", () => {
    const doc = makeDoc();
    // claims, concentration, excluded are optional — they have fallbacks in reshape
    expect(() => reshape(doc)).not.toThrow();
  });
});
