// Tests for item 6.4: daily cost-per-dollar line in the gas panel.
// RED first: written before the implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { rGasCostDaily } from "../app/assets/js/charts.js";

// ---------------------------------------------------------------------------
// Fixture factory
// ---------------------------------------------------------------------------

function baseDoc(costDaily) {
  const doc = {
    methodology_version: 1,
    generated_at: "2026-07-01T00:00:00Z",
    data_through_day: "2026-07-01",
    scope: "verified-x402",
    data: {
      windows: {
        all:  { txn_count: 10, volume_usdc: "100.00", by_band: { dust:{txn_count:2,volume_usdc:"0.02"}, micro:{txn_count:2,volume_usdc:"0.20"}, small:{txn_count:2,volume_usdc:"2.00"}, mid:{txn_count:2,volume_usdc:"2.00"}, whale:{txn_count:2,volume_usdc:"95.78"} } },
        "7d": { txn_count: 5,  volume_usdc: "50.00",  by_band: { dust:{txn_count:1,volume_usdc:"0.01"}, micro:{txn_count:1,volume_usdc:"0.10"}, small:{txn_count:1,volume_usdc:"1.00"}, mid:{txn_count:1,volume_usdc:"1.00"}, whale:{txn_count:1,volume_usdc:"47.89"} } },
        "30d":{ txn_count: 8,  volume_usdc: "80.00",  by_band: { dust:{txn_count:1,volume_usdc:"0.01"}, micro:{txn_count:1,volume_usdc:"0.10"}, small:{txn_count:2,volume_usdc:"2.00"}, mid:{txn_count:2,volume_usdc:"2.00"}, whale:{txn_count:2,volume_usdc:"75.89"} } },
      },
      daily_series: [{ day: "2026-07-01", txn_count: 10, volume_usdc: "100.00", complete: false }],
      monthly_series: [],
      typical_payment: {
        all:  { avg_usdc: "10.00", median_usdc: "5.00", txn_count: 10 },
        "7d": { avg_usdc: "10.00", median_usdc: "5.00", txn_count: 5 },
        "30d":{ avg_usdc: "10.00", median_usdc: "5.00", txn_count: 8 },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: {
        method: {},
        windows: {
          all:  { txn_count: 10, breakeven_txn_count: 5, gas_eth: "0.001", gas_eth_l1: "0", gas_eth_l2: "0.001", gas_usd: "2.00", gas_cents_per_dollar: "2.0000" },
          "7d": { txn_count: 5,  breakeven_txn_count: 2, gas_eth: "0.0005", gas_eth_l1: "0", gas_eth_l2: "0.0005", gas_usd: "1.00", gas_cents_per_dollar: "2.0000" },
          "30d":{ txn_count: 8,  breakeven_txn_count: 4, gas_eth: "0.0008", gas_eth_l1: "0", gas_eth_l2: "0.0008", gas_usd: "1.60", gas_cents_per_dollar: "2.0000" },
        },
        cost_daily: costDaily,
      },
      velocity: {
        windows: { all: { max_per_min: 5 }, "7d": { max_per_min: 3 }, "30d": { max_per_min: 4 } },
        daily_series: [],
      },
      claims: [],
      concentration: { windows: {} },
    },
  };
  return doc;
}

function gasDom() {
  document.body.innerHTML = `<div id="gascost-chart" style="width:800px"></div>`;
}

function setup(costDaily) {
  gasDom();
  const view = reshape(baseDoc(costDaily));
  setData(view);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
}

// ---------------------------------------------------------------------------
// Adapter: maps cost_daily to compact tuples
// ---------------------------------------------------------------------------

describe("item 6.4 - adapter: gas.cost_daily mapped to tuples", () => {
  it("maps [{day, cents_per_dollar, complete}] to [day, cents, complete] tuples", () => {
    const series = [
      { day: "2026-06-30", cents_per_dollar: "10.0000", complete: true },
      { day: "2026-07-01", cents_per_dollar: "4.0000",  complete: false },
    ];
    const view = reshape(baseDoc(series));
    expect(view.gas.cost_daily).toHaveLength(2);
    expect(view.gas.cost_daily[0]).toEqual(["2026-06-30", 10.0, true]);
    expect(view.gas.cost_daily[1]).toEqual(["2026-07-01", 4.0, false]);
  });

  it("missing cost_daily key produces empty array (old-artifact tolerance)", () => {
    const doc = baseDoc(undefined);
    delete doc.data.gas.cost_daily;
    let view;
    expect(() => { view = reshape(doc); }).not.toThrow();
    expect(view.gas.cost_daily).toEqual([]);
  });

  it("null cost_daily produces empty array", () => {
    const view = reshape(baseDoc(null));
    expect(view.gas.cost_daily).toEqual([]);
  });

  it("empty cost_daily array stays empty", () => {
    const view = reshape(baseDoc([]));
    expect(view.gas.cost_daily).toEqual([]);
  });

  it("complete flag is preserved faithfully (p.complete !== false convention)", () => {
    // An object without a complete field → treated as complete=true (same as daily tape).
    const series = [
      { day: "2026-06-01", cents_per_dollar: "5.0000" }, // no complete field
      { day: "2026-07-01", cents_per_dollar: "3.0000", complete: false },
    ];
    const view = reshape(baseDoc(series));
    expect(view.gas.cost_daily[0][2]).toBe(true);  // p.complete !== false → true
    expect(view.gas.cost_daily[1][2]).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// rGasCostDaily: renders chart
// ---------------------------------------------------------------------------

describe("item 6.4 - rGasCostDaily render", () => {
  it("renders SVG when cost_daily has data", () => {
    setup([
      { day: "2026-06-30", cents_per_dollar: "10.0000", complete: true },
      { day: "2026-07-01", cents_per_dollar: "4.0000",  complete: false },
    ]);
    rGasCostDaily();
    const html = document.getElementById("gascost-chart").innerHTML;
    expect(html).toContain("<svg");
  });

  it("renders no-data message when cost_daily is empty", () => {
    setup([]);
    rGasCostDaily();
    const html = document.getElementById("gascost-chart").innerHTML;
    expect(html).not.toContain("<svg");
    expect(html.length).toBeGreaterThan(0);
  });

  it("renders no-data message when cost_daily is absent (old artifact)", () => {
    const doc = baseDoc(undefined);
    delete doc.data.gas.cost_daily;
    const view = reshape(doc);
    setData(view);
    gasDom();
    rGasCostDaily();
    const html = document.getElementById("gascost-chart").innerHTML;
    expect(html).not.toContain("<svg");
  });

  it("marks the edge day (complete=false) as partial in rendered output", () => {
    setup([
      { day: "2026-06-30", cents_per_dollar: "10.0000", complete: true },
      { day: "2026-07-01", cents_per_dollar: "4.0000",  complete: false },
    ]);
    rGasCostDaily();
    const html = document.getElementById("gascost-chart").innerHTML;
    // Edge day must be visually distinguished (data-partial attribute or similar).
    expect(html).toMatch(/data-partial|partial|faint|opacity/i);
  });

  it("renders readout element inside chart host when data present", () => {
    setup([
      { day: "2026-06-30", cents_per_dollar: "10.0000", complete: true },
    ]);
    rGasCostDaily();
    const html = document.getElementById("gascost-chart").innerHTML;
    // Should have some interactive hover infrastructure (rect or similar).
    expect(html.length).toBeGreaterThan(50);
  });
});
