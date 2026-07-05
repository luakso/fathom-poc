// Tests for item 6.3: amount percentiles beyond the median in the PAYMENT SHAPE panel.
// RED first: written before the implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { rShape } from "../app/assets/js/panels.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";

// ---------------------------------------------------------------------------
// Shared fixture
// ---------------------------------------------------------------------------

function makeView(typicalOverride = {}) {
  return {
    meta: { methodology_version: 1, data_through_day: "2026-06-18", generated_at: "2026-06-18T00:00:00Z" },
    windows: {
      all: {
        txn_count: 100,
        volume_usdc: "100.00",
        by_band: {
          dust:  { txn_count: 20, volume_usdc: "0.20" },
          micro: { txn_count: 20, volume_usdc: "2.00" },
          small: { txn_count: 20, volume_usdc: "20.00" },
          mid:   { txn_count: 20, volume_usdc: "20.00" },
          whale: { txn_count: 20, volume_usdc: "57.80" },
        },
      },
      "7d":  { txn_count: 10, volume_usdc: "10.00", by_band: { dust:{txn_count:2,volume_usdc:"0.02"}, micro:{txn_count:2,volume_usdc:"0.20"}, small:{txn_count:2,volume_usdc:"2.00"}, mid:{txn_count:2,volume_usdc:"2.00"}, whale:{txn_count:2,volume_usdc:"5.78"} } },
      "30d": { txn_count: 50, volume_usdc: "50.00", by_band: { dust:{txn_count:10,volume_usdc:"0.10"}, micro:{txn_count:10,volume_usdc:"1.00"}, small:{txn_count:10,volume_usdc:"10.00"}, mid:{txn_count:10,volume_usdc:"10.00"}, whale:{txn_count:10,volume_usdc:"28.90"} } },
    },
    daily: [],
    monthly: [],
    typical: {
      all: {
        avg_usdc: "1.00",
        median_usdc: "0.10",
        txn_count: 100,
        ...typicalOverride,
      },
      "7d":  { avg_usdc: "1.00", median_usdc: "0.10", txn_count: 10 },
      "30d": { avg_usdc: "1.00", median_usdc: "0.10", txn_count: 50 },
    },
    price_points: { all: [], "7d": [], "30d": [] },
    gas: { windows: { all: { txn_count: 0, breakeven_txn_count: 0, gas_cents_per_dollar: null, gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0" }, "7d":{txn_count:0,breakeven_txn_count:0,gas_cents_per_dollar:null,gas_eth:"0",gas_eth_l1:"0",gas_eth_l2:"0",gas_usd:"0"}, "30d":{txn_count:0,breakeven_txn_count:0,gas_cents_per_dollar:null,gas_eth:"0",gas_eth_l1:"0",gas_eth_l2:"0",gas_usd:"0"} } },
    velocity: { windows: { all: { max_per_min: 0 }, "7d": { max_per_min: 0 }, "30d": { max_per_min: 0 } }, verified_daily: [] },
    claims: [],
    concentration: { windows: {} },
    excluded: null,
    active_entities: null,
  };
}

function shapeDom() {
  document.body.innerHTML = `
    <span id="shp-win"></span>
    <div id="shp-big"></div>
    <div id="bands"></div>`;
}

function setup(typicalOverride = {}) {
  shapeDom();
  setData(makeView(typicalOverride));
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
  state.bMetric = "tx";
  if (_clearPins) _clearPins();
}

// ---------------------------------------------------------------------------
// rShape: percentile strip renders when p10_usdc present
// ---------------------------------------------------------------------------

describe("item 6.3 - rShape percentile strip", () => {
  it("renders percentile strip when p10_usdc, p90_usdc, p99_usdc are present", () => {
    setup({ p10_usdc: "0.010000", p90_usdc: "0.900000", p99_usdc: "2.500000" });
    rShape();
    const html = document.getElementById("shp-big").innerHTML;
    // Strip should contain p10/p90/p99 labels and values.
    expect(html).toContain("p10");
    expect(html).toContain("p90");
    expect(html).toContain("p99");
    // Values should appear formatted (fmtAmt strips trailing zeros).
    expect(html).toContain("$0.01");
    expect(html).toContain("$0.9");
    expect(html).toContain("$2.5");
  });

  it("does NOT render percentile strip when p10_usdc is absent (old-artifact tolerance)", () => {
    setup(); // no p10/p90/p99 in typical
    rShape();
    const html = document.getElementById("shp-big").innerHTML;
    expect(html).not.toContain("p10");
    expect(html).not.toContain("p99");
  });

  it("does not crash when p10_usdc is null", () => {
    setup({ p10_usdc: null, p90_usdc: null, p99_usdc: null });
    expect(() => rShape()).not.toThrow();
    const html = document.getElementById("shp-big").innerHTML;
    expect(html).not.toContain("p10");
  });

  it("strip also contains the median value", () => {
    setup({ p10_usdc: "0.001000", p90_usdc: "0.500000", p99_usdc: "1.000000" });
    rShape();
    const html = document.getElementById("shp-big").innerHTML;
    // median_usdc = "0.10" → fmtAmt = "$0.1"
    expect(html).toContain("median");
    expect(html).toContain("$0.1");
  });
});

// ---------------------------------------------------------------------------
// PINNERS.shape: context includes percentile info when present
// ---------------------------------------------------------------------------

describe("item 6.3 - PINNERS.shape with percentiles", () => {
  it("context includes p10/p90/p99 values when present", () => {
    setup({ p10_usdc: "0.001000", p90_usdc: "0.500000", p99_usdc: "1.000000" });
    const pin = PINNERS.shape();
    expect(pin).not.toBeNull();
    expect(pin.context).toContain("p10");
    expect(pin.context).toContain("p90");
    expect(pin.context).toContain("p99");
  });

  it("context does NOT mention p10/p90/p99 when absent (old-artifact tolerance)", () => {
    setup();
    const pin = PINNERS.shape();
    expect(pin).not.toBeNull();
    expect(pin.context).not.toContain("p10");
    expect(pin.context).not.toContain("p99");
  });

  it("value is still the median when percentiles present", () => {
    setup({ p10_usdc: "0.001000", p90_usdc: "0.500000", p99_usdc: "1.000000" });
    const pin = PINNERS.shape();
    expect(pin.value).toContain("median");
    // median_usdc = "0.10" → fmtAmt → "$0.1"
    expect(pin.value).toContain("$0.1");
  });
});
