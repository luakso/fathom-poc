// Tests for item 6.1: active payers and payees per day (ACTIVE WALLETS panel).
// RED first: written before the implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rActiveWallets } from "../app/assets/js/charts.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function aeDoc(activeEntities) {
  return {
    methodology_version: 1,
    generated_at: "2026-06-06T00:00:00Z",
    data_through_day: "2026-06-06",
    scope: "verified-x402",
    data: {
      windows: {
        all:  { txn_count: 10, volume_usdc: "5.00", by_band: {} },
        "7d": { txn_count: 5,  volume_usdc: "2.50", by_band: {} },
        "30d":{ txn_count: 8,  volume_usdc: "4.00", by_band: {} },
      },
      daily_series: [{ day: "2026-06-06", txn_count: 10, volume_usdc: "5.00", complete: false }],
      monthly_series: [],
      typical_payment: {
        all:  { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 10 },
        "7d": { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 5 },
        "30d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 8 },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: { method: {}, windows: {
        all:  { txn_count: 10, breakeven_txn_count: 5, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "7d": { txn_count: 5,  breakeven_txn_count: 2, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "30d":{ txn_count: 8,  breakeven_txn_count: 4, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
      }},
      velocity: {
        windows: { all: { max_per_min: 0 }, "7d": { max_per_min: 0 }, "30d": { max_per_min: 0 } },
        daily_series: [],
      },
      claims: [],
      concentration: { windows: {} },
      active_entities: activeEntities,
    },
  };
}

function aecDom() {
  document.body.innerHTML = `<div id="aechart" style="width:800px"></div>`;
}

function setViewFromDoc(doc) {
  const view = reshape(doc);
  setData(view);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
}

// ---------------------------------------------------------------------------
// Adapter: pass-through and tolerance
// ---------------------------------------------------------------------------

describe("item 6.1 - adapter: active_entities pass-through", () => {
  it("reshapes active_entities from artifact into view.active_entities", () => {
    const series = [
      { day: "2026-06-01", complete: true,  payer_count: 5,  payee_count: 3 },
      { day: "2026-06-02", complete: false, payer_count: 2,  payee_count: 1 },
    ];
    const view = reshape(aeDoc(series));
    expect(view.active_entities).not.toBeNull();
    expect(view.active_entities).toHaveLength(2);
    expect(view.active_entities[0].payer_count).toBe(5);
    expect(view.active_entities[0].payee_count).toBe(3);
    expect(view.active_entities[1].complete).toBe(false);
  });

  it("missing active_entities key does not crash reshape (old artifact tolerance)", () => {
    const doc = aeDoc(undefined);
    delete doc.data.active_entities;
    let view;
    expect(() => { view = reshape(doc); }).not.toThrow();
    expect(view.active_entities).toBeNull();
  });

  it("null active_entities is tolerated", () => {
    const view = reshape(aeDoc(null));
    expect(view.active_entities).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// rActiveWallets: render
// ---------------------------------------------------------------------------

describe("item 6.1 - rActiveWallets render", () => {
  beforeEach(() => {
    aecDom();
    const series = [
      { day: "2026-06-01", complete: true,  payer_count: 12, payee_count: 5 },
      { day: "2026-06-02", complete: false, payer_count: 7,  payee_count: 4 },
    ];
    setViewFromDoc(aeDoc(series));
  });

  it("renders SVG inside #aechart when series is present", () => {
    rActiveWallets();
    const html = document.getElementById("aechart").innerHTML;
    expect(html).toContain("<svg");
  });

  it("encodes payer count in rendered output", () => {
    rActiveWallets();
    const html = document.getElementById("aechart").innerHTML;
    // max payer count 12 should appear as a tick label (fmtInt or fmtCount)
    expect(html).toContain("12");
  });

  it("renders a no-data message when active_entities is null", () => {
    const view = reshape(aeDoc(null));
    setData(view);
    rActiveWallets();
    const html = document.getElementById("aechart").innerHTML;
    expect(html).not.toContain("<svg");
    expect(html.length).toBeGreaterThan(0);
  });

  it("renders a no-data message when active_entities is empty", () => {
    const view = reshape(aeDoc([]));
    setData(view);
    rActiveWallets();
    const html = document.getElementById("aechart").innerHTML;
    expect(html).not.toContain("<svg");
  });

  it("marks the edge day dimmed (data-partial attribute or 'partial' class) when complete=false", () => {
    rActiveWallets();
    const html = document.getElementById("aechart").innerHTML;
    // Edge day (2026-06-02, complete=false) should be visually distinguished.
    expect(html).toMatch(/data-partial|partial|faint|opacity/i);
  });
});

// ---------------------------------------------------------------------------
// PINNERS.active_wallets
// ---------------------------------------------------------------------------

describe("item 6.1 - PINNERS.active_wallets", () => {
  beforeEach(() => {
    if (_clearPins) _clearPins();
  });

  it("returns a pin with title ACTIVE WALLETS", () => {
    const series = [
      { day: "2026-06-01", complete: true,  payer_count: 50, payee_count: 20 },
      { day: "2026-06-02", complete: false, payer_count: 10, payee_count: 8 },
    ];
    const view = reshape(aeDoc(series));
    setData(view);
    const pin = PINNERS.active_wallets();
    expect(pin).not.toBeNull();
    expect(pin.title).toContain("ACTIVE WALLETS");
  });

  it("pin value mentions payer and payee counts from the last complete day", () => {
    const series = [
      { day: "2026-06-01", complete: true,  payer_count: 50, payee_count: 20 },
      { day: "2026-06-02", complete: false, payer_count: 10, payee_count: 8 },
    ];
    const view = reshape(aeDoc(series));
    setData(view);
    const pin = PINNERS.active_wallets();
    // Should mention 50 payers and 20 payees (last complete day)
    expect(pin.value).toContain("50");
    expect(pin.value).toContain("20");
  });

  it("pin denom mentions verified payments", () => {
    const series = [
      { day: "2026-06-01", complete: true, payer_count: 5, payee_count: 3 },
    ];
    const view = reshape(aeDoc(series));
    setData(view);
    const pin = PINNERS.active_wallets();
    expect(pin.denom).toContain("verified");
  });

  it("returns null when active_entities is missing", () => {
    const view = reshape(aeDoc(null));
    setData(view);
    const pin = PINNERS.active_wallets();
    expect(pin).toBeNull();
  });

  it("returns null when active_entities is empty", () => {
    const view = reshape(aeDoc([]));
    setData(view);
    const pin = PINNERS.active_wallets();
    expect(pin).toBeNull();
  });

  it("pin exports a series array for sparkline", () => {
    const series = [
      { day: "2026-06-01", complete: true,  payer_count: 50, payee_count: 20 },
      { day: "2026-06-02", complete: false, payer_count: 10, payee_count: 8 },
    ];
    const view = reshape(aeDoc(series));
    setData(view);
    const pin = PINNERS.active_wallets();
    expect(Array.isArray(pin.series)).toBe(true);
    expect(pin.series.length).toBe(2);
  });
});
