// Tests for item 5.1: partial-day flag on the daily tape.
// Covers adapter reshape, tray PINNERS.daily, and rDaily rendering.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rDaily } from "../app/assets/js/charts.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function dailyDom() {
  document.body.innerHTML = `
    <div id="dailychart" style="width:900px"></div>
    <span id="d-range"></span>
    <span id="d-readout"></span>
    <span id="d-partial-note" style="display:none"></span>`;
}

function makeRawDoc(dailySeries) {
  return {
    methodology_version: 1,
    generated_at: "2026-06-06T00:00:00Z",
    data_through_day: "2026-06-06",
    scope: "verified-x402",
    data: {
      windows: {
        all:  { txn_count: 100, volume_usdc: "50.00", by_band: {} },
        "7d": { txn_count: 50,  volume_usdc: "25.00", by_band: {} },
        "30d":{ txn_count: 80,  volume_usdc: "40.00", by_band: {} },
      },
      daily_series: dailySeries,
      monthly_series: [],
      typical_payment: {
        all: { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 100 },
        "7d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 50 },
        "30d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 80 },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: { method: {}, windows: {
        all:  { txn_count: 100, breakeven_txn_count: 50, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "7d": { txn_count: 50,  breakeven_txn_count: 25, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "30d":{ txn_count: 80,  breakeven_txn_count: 40, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
      }},
      velocity: {
        windows: { all: { max_per_min: 0 }, "7d": { max_per_min: 0 }, "30d": { max_per_min: 0 } },
        daily_series: [],
      },
      claims: [],
      concentration: { windows: {} },
    },
  };
}

// Shared view setup for tray tests.
function makeViewFromRaw(dailySeries) {
  return reshape(makeRawDoc(dailySeries));
}

// ---------------------------------------------------------------------------
// Part 1 — adapter.js reshape: complete flag in daily tuples
// ---------------------------------------------------------------------------
describe("partial-day flag — adapter.js reshape", () => {
  it("passes complete=true for all but the last day", () => {
    const ds = [
      { day: "2026-06-04", txn_count: 140000, volume_usdc: "700.000000", complete: true },
      { day: "2026-06-05", txn_count: 138000, volume_usdc: "690.000000", complete: true },
      { day: "2026-06-06", txn_count:   2138, volume_usdc:  "10.000000", complete: false },
    ];
    const view = makeViewFromRaw(ds);
    expect(view.daily[0][3]).toBe(true,  "first day must be complete");
    expect(view.daily[1][3]).toBe(true,  "middle day must be complete");
    expect(view.daily[2][3]).toBe(false, "last (edge) day must be incomplete");
  });

  it("backward compat: artifact without complete field treats all points as complete", () => {
    const ds = [
      { day: "2026-06-04", txn_count: 140000, volume_usdc: "700.000000" },
      { day: "2026-06-05", txn_count: 138000, volume_usdc: "690.000000" },
      // no `complete` field
    ];
    const view = makeViewFromRaw(ds);
    expect(view.daily[0][3]).toBe(true);
    expect(view.daily[1][3]).toBe(true);
  });

  it("tuple still has day at index 0, txn_count at 1, volume at 2 (existing consumers unchanged)", () => {
    const ds = [
      { day: "2026-06-06", txn_count: 2138, volume_usdc: "10.700000", complete: false },
    ];
    const view = makeViewFromRaw(ds);
    const t = view.daily[0];
    expect(t[0]).toBe("2026-06-06");
    expect(t[1]).toBe(2138);
    expect(typeof t[2]).toBe("number"); // rounded volume
    expect(t[3]).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Part 2 — tray.js PINNERS.daily: peak skips incomplete points
// ---------------------------------------------------------------------------

// 10-day series: the LAST day (2026-06-06) is incomplete and has the highest
// tx count (999999) AND the highest USD volume (99999). Peak selection must
// skip it and pick the true peak from complete days only.
const incompleteIsHighestDaily = [
  ["2026-05-28", 5000, 500.00, true],
  ["2026-05-29", 4000, 400.00, true],
  ["2026-05-30", 3500, 350.00, true],
  ["2026-05-31", 3000, 300.00, true],
  ["2026-06-01", 2500, 250.00, true],
  ["2026-06-02", 2000, 200.00, true],
  ["2026-06-03", 1500, 150.00, true],
  ["2026-06-04", 1000, 100.00, true],
  ["2026-06-05",  900,  90.00, true],
  ["2026-06-06", 999999, 99999.00, false],  // incomplete edge — must be excluded from peak
];

const mockDataForPeakTests = {
  meta: { data_through_day: "2026-06-06", methodology_version: 1 },
  windows: {
    all:  { txn_count: 100, volume_usdc: "100.00", by_band: {} },
    "7d": { txn_count: 50,  volume_usdc: "50.00",  by_band: {} },
    "30d":{ txn_count: 80,  volume_usdc: "80.00",  by_band: {} },
  },
  daily: incompleteIsHighestDaily,
  monthly: [],
  typical: {
    all:  { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 100 },
    "7d": { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 50 },
    "30d":{ avg_usdc: "1.00", median_usdc: "0.50", txn_count: 80 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 100, breakeven_txn_count: 50, gas_cents_per_dollar: "0.50" },
    "7d": { txn_count: 50,  breakeven_txn_count: 25, gas_cents_per_dollar: "0.50" },
    "30d":{ txn_count: 80,  breakeven_txn_count: 40, gas_cents_per_dollar: "0.50" },
  }},
  velocity: {
    windows: { all: { max_per_min: 0 }, "7d": { max_per_min: 0 }, "30d": { max_per_min: 0 } },
    verified_daily: [],
  },
  claims: [],
  concentration: { windows: {} },
};

describe("partial-day flag — PINNERS.daily peak exclusion", () => {
  let PINNERS_mod;

  beforeEach(async () => {
    setData(mockDataForPeakTests);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    state.dWin = "all";
    state.dMetric = "tx";
    state.dMa = "raw";
    state.dScale = "lin";
    const m = await import("../app/assets/js/tray.js");
    PINNERS_mod = m;
    if (m._clearPins) m._clearPins();
  });

  it("tx mode: peak skips the incomplete edge day (tx=999999 excluded, picks 5000 from complete)", () => {
    state.dMetric = "tx";
    const pin = PINNERS_mod.PINNERS.daily();
    expect(pin).not.toBeNull();
    // Must NOT contain 999999 (incomplete day's count)
    expect(pin.value).not.toContain("999,999");
    // Must contain 5000 (highest complete-day tx count)
    expect(pin.value).toContain("5,000");
    // Day of the peak must be 2026-05-28
    expect(pin.context).toContain("2026-05-28");
  });

  it("usd mode: peak skips the incomplete edge day (usd=99999 excluded, picks 500 from complete)", () => {
    state.dMetric = "usd";
    const pin = PINNERS_mod.PINNERS.daily();
    expect(pin).not.toBeNull();
    // Must NOT contain 99999 (incomplete day's volume)
    expect(pin.value).not.toContain("99,999");
    // Must contain 500 (highest complete-day volume)
    expect(pin.context).toContain("2026-05-28");
  });

  it("denom notes partial edge day when the slice contains one incomplete point", () => {
    const pin = PINNERS_mod.PINNERS.daily();
    expect(pin).not.toBeNull();
    expect(pin.denom).toContain("partial");
  });

  it("denom has no partial note when ALL points are complete (backward compat artifact)", () => {
    // Replace the daily series with all-complete points
    const allComplete = incompleteIsHighestDaily.map(d => [d[0], d[1], d[2], true]);
    const completeData = { ...mockDataForPeakTests, daily: allComplete };
    setData(completeData);
    const pin = PINNERS_mod.PINNERS.daily();
    expect(pin).not.toBeNull();
    expect(pin.denom).not.toContain("partial");
  });
});

// ---------------------------------------------------------------------------
// Part 3 — charts.js rDaily: does not crash; renders distinct marker for edge
// ---------------------------------------------------------------------------
describe("partial-day flag — rDaily rendering", () => {
  beforeEach(() => {
    dailyDom();
    setData(mockDataForPeakTests);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    state.dWin = "all";
    state.dMetric = "tx";
    state.dMa = "raw";
    state.dScale = "lin";
  });

  it("does not throw when last point is incomplete", () => {
    expect(() => rDaily()).not.toThrow();
  });

  it("renders the daily chart SVG (path exists)", () => {
    rDaily();
    const svg = document.querySelector("#dailychart svg");
    expect(svg).not.toBeNull();
    const paths = svg.querySelectorAll("path");
    expect(paths.length).toBeGreaterThan(0);
  });

  it("renders a distinct marker for the incomplete edge point", () => {
    rDaily();
    const svg = document.querySelector("#dailychart svg");
    // The incomplete edge point should have a dedicated element (circle or similar)
    // with a class or id indicating it's the partial-day marker.
    const marker = svg.querySelector("[data-partial='true'], .partial-edge, #partial-edge");
    expect(marker).not.toBeNull();
  });

  it("MA7 mode: renders without NaN in SVG path data", () => {
    state.dMa = "ma7";
    rDaily();
    const svg = document.querySelector("#dailychart svg");
    expect(svg).not.toBeNull();
    const html = svg.innerHTML;
    expect(html).not.toContain("NaN");
  });
});
