// Tests for 6.7 - price-point breadth: sparklines render in rPrice, absence tolerance.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { rPrice } from "../app/assets/js/panels.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";

// ---------------------------------------------------------------------------
// Shared DOM for the price panel.
// ---------------------------------------------------------------------------
function priceDom() {
  document.body.innerHTML = `
    <span id="pp-win"></span>
    <table id="pptable"></table>`;
}

// Shared minimal view-model with price_points and optional price_breadth.
function makeData(priceBreadth = null) {
  return {
    meta: { data_through_day: "2026-06-08", methodology_version: 1, generated_at: "2026-06-08T00:00:00Z" },
    windows: {
      all:  { txn_count: 1000, volume_usdc: "500.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:1000,volume_usdc:"500.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
      "7d": { txn_count: 200, volume_usdc: "100.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:200,volume_usdc:"100.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
      "30d":{ txn_count: 600, volume_usdc: "300.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:600,volume_usdc:"300.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
    },
    daily: [],
    monthly: [],
    typical: {
      all:  { avg_usdc: "0.50", median_usdc: "0.001", txn_count: 1000 },
      "7d": { avg_usdc: "0.50", median_usdc: "0.001", txn_count: 200 },
      "30d":{ avg_usdc: "0.50", median_usdc: "0.001", txn_count: 600 },
    },
    price_points: {
      all: [
        { amount_usdc: "0.001000", txn_count: 800, volume_usdc: "0.800000", payee_count: 50, txn_share_pct: "80.0" },
        { amount_usdc: "1.000000", txn_count: 200, volume_usdc: "200.000000", payee_count: 5,  txn_share_pct: "20.0" },
      ],
      "7d": [
        { amount_usdc: "0.001000", txn_count: 150, volume_usdc: "0.150000", payee_count: 30, txn_share_pct: "75.0" },
      ],
      "30d": [
        { amount_usdc: "0.001000", txn_count: 500, volume_usdc: "0.500000", payee_count: 40, txn_share_pct: "83.3" },
      ],
    },
    // price_breadth is the reshaped name of price_point_breadth.
    price_breadth: priceBreadth,
    gas: { windows: {
      all:  { txn_count: 1000, breakeven_txn_count: 500, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
      "7d": { txn_count: 200,  breakeven_txn_count: 100, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
      "30d":{ txn_count: 600,  breakeven_txn_count: 300, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
    }},
    velocity: { windows: { all:{max_per_min:5},"7d":{max_per_min:2},"30d":{max_per_min:4} }, verified_daily: [] },
    claims: [],
    concentration: { windows: {} },
    excluded: null,
    active_entities: null,
    payer_cohorts: null,
  };
}

// A price_breadth value with two price points and a multi-day series.
const mockBreadth = [
  {
    amount_usdc: "0.001000",
    series: [
      { day: "2026-06-01", payee_count: 10, complete: true },
      { day: "2026-06-05", payee_count: 15, complete: true },
      { day: "2026-06-08", payee_count: 20, complete: false }, // edge day
    ],
  },
  {
    amount_usdc: "1.000000",
    series: [
      { day: "2026-06-07", payee_count: 3, complete: true },
      { day: "2026-06-08", payee_count: 4, complete: false },
    ],
  },
];

// ---------------------------------------------------------------------------
// rPrice: sparklines rendered when price_breadth is present
// ---------------------------------------------------------------------------
describe("6.7 rPrice — sparklines when price_breadth is present", () => {
  beforeEach(() => {
    priceDom();
    setData(makeData(mockBreadth));
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("renders the price table normally (existing columns still present)", () => {
    rPrice();
    const html = document.getElementById("pptable").innerHTML;
    expect(html).toContain("amount");
    expect(html).toContain("payees");
    expect(html).toContain("MENU");
  });

  it("renders inline SVG sparklines for price points with series data", () => {
    rPrice();
    const html = document.getElementById("pptable").innerHTML;
    expect(html).toContain("<svg");
    expect(html).toContain("<polyline");
  });

  it("sparkline uses var(--agentic) stroke color", () => {
    rPrice();
    const html = document.getElementById("pptable").innerHTML;
    expect(html).toContain("var(--agentic)");
  });

  it("adds a 'payees trend' column header when breadth is present", () => {
    rPrice();
    const html = document.getElementById("pptable").innerHTML;
    expect(html).toContain("payees trend");
  });
});

// ---------------------------------------------------------------------------
// rPrice: old-artifact tolerance when price_breadth is absent
// ---------------------------------------------------------------------------
describe("6.7 rPrice — absence tolerance (no price_breadth)", () => {
  beforeEach(() => {
    priceDom();
    setData(makeData(null)); // no price_breadth
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("renders the price table without sparklines when price_breadth is null", () => {
    rPrice();
    const html = document.getElementById("pptable").innerHTML;
    expect(html).toContain("amount");
    expect(html).not.toContain("<svg");
    expect(html).not.toContain("payees trend");
  });

  it("does not throw when price_breadth is null", () => {
    expect(() => rPrice()).not.toThrow();
  });

  it("price pinner still works without price_breadth (no trendCtx crash)", () => {
    state.win = "all";
    expect(() => PINNERS.price()).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// PINNERS.price: payees trend appended when data supports it
// ---------------------------------------------------------------------------
describe("6.7 PINNERS.price — trend direction in pin context", () => {
  beforeEach(() => {
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("appends 'payees trend: widening' when last > first by >= 20%", () => {
    // first=10, last=20 → +100% → widening
    const breadth = [{
      amount_usdc: "0.001000",
      series: [
        { day: "2026-06-01", payee_count: 10, complete: true },
        { day: "2026-06-08", payee_count: 20, complete: false }, // edge: excluded from trend
        // But we need a complete point after the first...
        // Actually, the edge day is complete:false, so trend uses only complete points.
        // Let's add an intermediate complete point.
      ],
    }];
    // Rebuild: first=10 (complete), then edge (incomplete). Only 1 complete point → no trend.
    // Let's use 2 complete points.
    const breadthWithTrend = [{
      amount_usdc: "0.001000",
      series: [
        { day: "2026-06-01", payee_count: 10, complete: true },
        { day: "2026-06-07", payee_count: 20, complete: true },
        { day: "2026-06-08", payee_count: 25, complete: false },
      ],
    }];
    setData(makeData(breadthWithTrend));
    const pin = PINNERS.price();
    expect(pin.context).toContain("payees trend: widening");
  });

  it("appends 'payees trend: narrowing' when last < first by >= 20%", () => {
    const breadth = [{
      amount_usdc: "0.001000",
      series: [
        { day: "2026-06-01", payee_count: 100, complete: true },
        { day: "2026-06-07", payee_count: 50, complete: true },
        { day: "2026-06-08", payee_count: 48, complete: false },
      ],
    }];
    setData(makeData(breadth));
    const pin = PINNERS.price();
    expect(pin.context).toContain("payees trend: narrowing");
  });

  it("appends 'payees trend: flat' when change is within ±20%", () => {
    const breadth = [{
      amount_usdc: "0.001000",
      series: [
        { day: "2026-06-01", payee_count: 100, complete: true },
        { day: "2026-06-07", payee_count: 110, complete: true }, // +10% → flat
        { day: "2026-06-08", payee_count: 108, complete: false },
      ],
    }];
    setData(makeData(breadth));
    const pin = PINNERS.price();
    expect(pin.context).toContain("payees trend: flat");
  });

  it("omits trend when only 1 complete day exists", () => {
    const breadth = [{
      amount_usdc: "0.001000",
      series: [
        { day: "2026-06-08", payee_count: 50, complete: false }, // edge only, no complete days
      ],
    }];
    setData(makeData(breadth));
    const pin = PINNERS.price();
    expect(pin.context).not.toContain("payees trend");
  });

  it("omits trend when price_breadth is null", () => {
    setData(makeData(null));
    const pin = PINNERS.price();
    expect(pin).not.toBeNull();
    expect(pin.context).not.toContain("payees trend");
  });

  it("pin context still contains the standard price-points text", () => {
    setData(makeData(null));
    const pin = PINNERS.price();
    expect(pin.context).toContain("80.0%");
    expect(pin.context).toContain("payees");
  });
});
