// Tests for item 6.2: largest payment per window.
// RED first: written before the implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rOverview } from "../app/assets/js/panels.js";

// ---------------------------------------------------------------------------
// Shared fixture factory
// ---------------------------------------------------------------------------

function makeDoc(typicalOverride = {}) {
  return {
    methodology_version: 1,
    generated_at: "2026-07-01T00:00:00Z",
    data_through_day: "2026-07-01",
    scope: "verified-x402",
    data: {
      windows: {
        all:  { txn_count: 100, volume_usdc: "5000.00", by_band: { dust:{txn_count:20,volume_usdc:"0.20"}, micro:{txn_count:20,volume_usdc:"2.00"}, small:{txn_count:20,volume_usdc:"20.00"}, mid:{txn_count:20,volume_usdc:"20.00"}, whale:{txn_count:20,volume_usdc:"4957.80"} } },
        "7d": { txn_count: 10,  volume_usdc: "500.00",  by_band: { dust:{txn_count:2,volume_usdc:"0.02"}, micro:{txn_count:2,volume_usdc:"0.20"}, small:{txn_count:2,volume_usdc:"2.00"}, mid:{txn_count:2,volume_usdc:"2.00"}, whale:{txn_count:2,volume_usdc:"495.78"} } },
        "30d":{ txn_count: 50,  volume_usdc: "2500.00", by_band: { dust:{txn_count:10,volume_usdc:"0.10"}, micro:{txn_count:10,volume_usdc:"1.00"}, small:{txn_count:10,volume_usdc:"10.00"}, mid:{txn_count:10,volume_usdc:"10.00"}, whale:{txn_count:10,volume_usdc:"2478.90"} } },
      },
      daily_series: [{ day: "2026-07-01", txn_count: 100, volume_usdc: "5000.00", complete: false }],
      monthly_series: [],
      typical_payment: {
        all:  { avg_usdc: "50.00", median_usdc: "10.00", txn_count: 100, ...typicalOverride },
        "7d": { avg_usdc: "50.00", median_usdc: "10.00", txn_count: 10, ...typicalOverride },
        "30d":{ avg_usdc: "50.00", median_usdc: "10.00", txn_count: 50, ...typicalOverride },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: { method: {}, windows: {
        all:  { txn_count: 100, breakeven_txn_count: 50, gas_eth: "0.001", gas_eth_l1: "0", gas_eth_l2: "0.001", gas_usd: "2.00", gas_cents_per_dollar: "0.0400" },
        "7d": { txn_count: 10,  breakeven_txn_count: 5,  gas_eth: "0.0001", gas_eth_l1: "0", gas_eth_l2: "0.0001", gas_usd: "0.20", gas_cents_per_dollar: "0.0400" },
        "30d":{ txn_count: 50,  breakeven_txn_count: 25, gas_eth: "0.0005", gas_eth_l1: "0", gas_eth_l2: "0.0005", gas_usd: "1.00", gas_cents_per_dollar: "0.0400" },
      }},
      velocity: {
        windows: { all: { max_per_min: 10 }, "7d": { max_per_min: 5 }, "30d": { max_per_min: 8 } },
        daily_series: [],
      },
      claims: [],
      concentration: { windows: {} },
    },
  };
}

function ovDom() {
  document.body.innerHTML = `
    <span id="ov-win"></span>
    <div id="ov-stats"></div>
    <div id="ov-bands"></div>
    <div id="ov-denom"></div>`;
}

function setup(typicalOverride = {}) {
  ovDom();
  const view = reshape(makeDoc(typicalOverride));
  setData(view);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
  if (_clearPins) _clearPins();
}

// ---------------------------------------------------------------------------
// Adapter: pass-through
// ---------------------------------------------------------------------------

describe("item 6.2 - adapter: largest_payment_usdc pass-through", () => {
  it("view.typical[all].largest_payment_usdc reflects artifact value", () => {
    const view = reshape(makeDoc({ largest_payment_usdc: "12345.000000" }));
    expect(view.typical.all.largest_payment_usdc).toBe("12345.000000");
  });

  it("missing largest_payment_usdc does not crash reshape (old-artifact tolerance)", () => {
    let view;
    expect(() => { view = reshape(makeDoc()); }).not.toThrow();
    // field simply absent / undefined
    expect(view.typical.all.largest_payment_usdc).toBeUndefined();
  });

  it("null largest_payment_usdc is tolerated", () => {
    const view = reshape(makeDoc({ largest_payment_usdc: null }));
    expect(view.typical.all.largest_payment_usdc).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// rOverview: renders compact stat
// ---------------------------------------------------------------------------

describe("item 6.2 - rOverview largest payment stat", () => {
  it("renders largest stat when largest_payment_usdc is present", () => {
    // Use a value that formats as "$99.00" — unambiguous in the output.
    setup({ largest_payment_usdc: "99.000000" });
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    // The label "LARGEST PAYMENT" must appear.
    expect(html.toUpperCase()).toContain("LARGEST PAYMENT");
    // fmtMoney("99.000000") → "$99.00"
    expect(html).toContain("99.00 USDC");
  });

  it("omits largest stat when largest_payment_usdc is absent (old-artifact tolerance)", () => {
    setup();  // no largest_payment_usdc
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    expect(html.toLowerCase()).not.toContain("largest");
  });

  it("omits largest stat when largest_payment_usdc is null", () => {
    setup({ largest_payment_usdc: null });
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    expect(html.toLowerCase()).not.toContain("largest");
  });
});

// ---------------------------------------------------------------------------
// PINNERS.overview: carries largest in context
// ---------------------------------------------------------------------------

describe("item 6.2 - PINNERS.overview largest in context", () => {
  it("context includes 'largest' when largest_payment_usdc is present", () => {
    setup({ largest_payment_usdc: "4957.800000" });
    const pin = PINNERS.overview();
    expect(pin).not.toBeNull();
    expect(pin.context.toLowerCase()).toContain("largest");
  });

  it("context does NOT mention 'largest' when absent (old-artifact tolerance)", () => {
    setup();
    const pin = PINNERS.overview();
    expect(pin).not.toBeNull();
    expect(pin.context.toLowerCase()).not.toContain("largest");
  });

  it("context largest value is formatted as money (not raw decimal)", () => {
    setup({ largest_payment_usdc: "4957.800000" });
    const pin = PINNERS.overview();
    // fmtMoney(4957.8) → "$5.0k"; NOT the raw string "4957.800000"
    expect(pin.context).not.toContain("4957.800000");
    expect(pin.context).toContain("USDC");
  });
});
