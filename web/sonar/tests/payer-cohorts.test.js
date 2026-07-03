// Tests for item 6.5: new vs returning payer cohort breakdown.
// RED first: written before the implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { reshape } from "../app/assets/js/adapter.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rPayerCohorts } from "../app/assets/js/panels.js";
import { num } from "../app/assets/js/format.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function cohortDoc(payerCohorts) {
  return {
    methodology_version: 1,
    generated_at: "2026-06-06T00:00:00Z",
    data_through_day: "2026-06-06",
    scope: "verified-x402",
    data: {
      windows: {
        all:  { txn_count: 10, volume_usdc: "12.00", by_band: {} },
        "7d": { txn_count: 7,  volume_usdc: "8.00",  by_band: {} },
        "30d":{ txn_count: 9,  volume_usdc: "11.00", by_band: {} },
      },
      daily_series: [{ day: "2026-06-06", txn_count: 10, volume_usdc: "12.00", complete: false }],
      monthly_series: [],
      typical_payment: {
        all:  { avg_usdc: "1.20", median_usdc: "1.00", txn_count: 10 },
        "7d": { avg_usdc: "1.14", median_usdc: "1.00", txn_count: 7 },
        "30d":{ avg_usdc: "1.22", median_usdc: "1.00", txn_count: 9 },
      },
      price_points: { all: [], "7d": [], "30d": [] },
      gas: { method: {}, windows: {
        all:  { txn_count: 10, breakeven_txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "7d": { txn_count: 7,  breakeven_txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
        "30d":{ txn_count: 9,  breakeven_txn_count: 0, gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", gas_cents_per_dollar: null },
      }},
      velocity: {
        windows: { all: { max_per_min: 0 }, "7d": { max_per_min: 0 }, "30d": { max_per_min: 0 } },
        daily_series: [],
      },
      claims: [],
      concentration: { windows: {} },
      active_entities: [
        { day: "2026-06-05", complete: true,  payer_count: 3, payee_count: 2 },
        { day: "2026-06-06", complete: false, payer_count: 2, payee_count: 1 },
      ],
      // new_payer_volume_usdc + returning_payer_volume_usdc must equal the window volume
      payer_cohorts: payerCohorts,
    },
  };
}

// Fixture cohorts where new+ret volume equals window volume for both windows.
// 7d window vol = 8.00 -> new=5.00, ret=3.00; 30d window vol = 11.00 -> new=7.00, ret=4.00
const FIXTURE_COHORTS = {
  "7d":  { new_payers: 2, returning_payers: 1, new_payer_volume_usdc: "5.000000", returning_payer_volume_usdc: "3.000000" },
  "30d": { new_payers: 3, returning_payers: 2, new_payer_volume_usdc: "7.000000", returning_payer_volume_usdc: "4.000000" },
};

function cohortDom() {
  document.body.innerHTML = `<div id="ae-cohort"></div>`;
}

function setViewFromDoc(doc) {
  const view = reshape(doc);
  setData(view);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
}

// ---------------------------------------------------------------------------
// Adapter: payer_cohorts pass-through
// ---------------------------------------------------------------------------

describe("item 6.5 - adapter: payer_cohorts pass-through", () => {
  it("reshapes payer_cohorts from artifact into view.payer_cohorts", () => {
    const view = reshape(cohortDoc(FIXTURE_COHORTS));
    expect(view.payer_cohorts).not.toBeNull();
    expect(view.payer_cohorts["7d"]).toBeDefined();
    expect(view.payer_cohorts["7d"].new_payers).toBe(2);
    expect(view.payer_cohorts["7d"].returning_payers).toBe(1);
    expect(view.payer_cohorts["30d"].new_payers).toBe(3);
  });

  it("missing payer_cohorts key gives null in view (old-artifact tolerance)", () => {
    const doc = cohortDoc(undefined);
    delete doc.data.payer_cohorts;
    const view = reshape(doc);
    expect(view.payer_cohorts).toBeNull();
  });

  it("null payer_cohorts is tolerated", () => {
    const view = reshape(cohortDoc(null));
    expect(view.payer_cohorts).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// rPayerCohorts: render
// ---------------------------------------------------------------------------

describe("item 6.5 - rPayerCohorts render", () => {
  beforeEach(() => {
    cohortDom();
  });

  it("renders two-segment bars for 7d and 30d when data is present", () => {
    setViewFromDoc(cohortDoc(FIXTURE_COHORTS));
    rPayerCohorts();
    const html = document.getElementById("ae-cohort").innerHTML;
    expect(html).toContain("7d");
    expect(html).toContain("30d");
    // Both new and returning segments must be rendered
    expect(html).toContain("new");
    expect(html).toContain("returning");
  });

  it("renders nothing (empty string) when payer_cohorts is null", () => {
    setViewFromDoc(cohortDoc(null));
    rPayerCohorts();
    const html = document.getElementById("ae-cohort").innerHTML.trim();
    expect(html).toBe("");
  });

  it("shows correct new% derived from new_payer_volume_usdc / total vol", () => {
    setViewFromDoc(cohortDoc(FIXTURE_COHORTS));
    rPayerCohorts();
    const html = document.getElementById("ae-cohort").innerHTML;
    // 7d: new=5.00 / (5.00+3.00) = 62.5%
    const expectedPct = (5 / 8 * 100).toFixed(1);
    expect(html).toContain(expectedPct);
  });
});

// ---------------------------------------------------------------------------
// Volume conservation assertion
// ---------------------------------------------------------------------------

describe("item 6.5 - conservation: new_vol + ret_vol == window_vol", () => {
  it("new_payer_volume_usdc + returning_payer_volume_usdc equals window volume for 7d", () => {
    const view = reshape(cohortDoc(FIXTURE_COHORTS));
    const c7 = view.payer_cohorts["7d"];
    const sum = num(c7.new_payer_volume_usdc) + num(c7.returning_payer_volume_usdc);
    const windowVol = num(view.windows["7d"].volume_usdc);
    expect(Math.abs(sum - windowVol)).toBeLessThan(0.000001);
  });

  it("new_payer_volume_usdc + returning_payer_volume_usdc equals window volume for 30d", () => {
    const view = reshape(cohortDoc(FIXTURE_COHORTS));
    const c30 = view.payer_cohorts["30d"];
    const sum = num(c30.new_payer_volume_usdc) + num(c30.returning_payer_volume_usdc);
    const windowVol = num(view.windows["30d"].volume_usdc);
    expect(Math.abs(sum - windowVol)).toBeLessThan(0.000001);
  });
});

// ---------------------------------------------------------------------------
// PINNERS.active_wallets: cohort context
// ---------------------------------------------------------------------------

describe("item 6.5 - PINNERS.active_wallets cohort context", () => {
  beforeEach(() => {
    if (_clearPins) _clearPins();
  });

  it("when cohorts are present, pin context includes 7d new% number", () => {
    setViewFromDoc(cohortDoc(FIXTURE_COHORTS));
    const pin = PINNERS.active_wallets();
    expect(pin).not.toBeNull();
    // 7d: new=5.00 / (5.00+3.00) = 62.5% new
    const expected7dNewPct = (5 / 8 * 100).toFixed(1);
    expect(pin.context).toContain(expected7dNewPct);
  });

  it("when cohorts are null, pin still returns (not null) with existing behavior", () => {
    setViewFromDoc(cohortDoc(null));
    const pin = PINNERS.active_wallets();
    // active_entities is present, so pin should not be null
    expect(pin).not.toBeNull();
    expect(pin.title).toContain("ACTIVE WALLETS");
    // context should not mention cohort percentages
    expect(pin.context).not.toContain("new");
  });

  it("returns null when active_entities is absent (no panel data)", () => {
    const doc = cohortDoc(FIXTURE_COHORTS);
    doc.data.active_entities = null;
    setViewFromDoc(doc);
    const pin = PINNERS.active_wallets();
    expect(pin).toBeNull();
  });
});
