// Tests for presentation-honesty fixes (items 3.1, 3.2, 3.3).
// TDD: written RED-first before implementation.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel, setIssues } from "../app/assets/js/state.js";
import { rOverview, rShape, rShell } from "../app/assets/js/panels.js";
import { checkIntegrity } from "../app/assets/js/adapter.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";

// ---------------------------------------------------------------------------
// Shared fixture — all band sums equal their window totals (healthy artifact)
// ---------------------------------------------------------------------------
const baseView = {
  meta: { methodology_version: 1, data_through_day: "2026-06-18", generated_at: "2026-06-18T00:00:00Z" },
  windows: {
    all: {
      txn_count: 21188494,
      volume_usdc: "7538241.50",
      by_band: {
        dust:  { txn_count: 13000000, volume_usdc: "10000.00" },
        micro: { txn_count: 3000000,  volume_usdc: "100000.00" },
        small: { txn_count: 2000000,  volume_usdc: "500000.00" },
        mid:   { txn_count: 500000,   volume_usdc: "2000000.00" },
        whale: { txn_count: 2688494,  volume_usdc: "4928241.50" },
      },
    },
    "7d": {
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
    "30d": {
      txn_count: 500,
      volume_usdc: "200.00",
      by_band: {
        dust:  { txn_count: 100, volume_usdc: "1.00" },
        micro: { txn_count: 100, volume_usdc: "10.00" },
        small: { txn_count: 100, volume_usdc: "40.00" },
        mid:   { txn_count: 100, volume_usdc: "49.00" },
        whale: { txn_count: 100, volume_usdc: "100.00" },
      },
    },
  },
  daily: [["2026-06-18", 100, 50.0]],
  monthly: [],
  typical: {
    all:  { avg_usdc: "0.355771", median_usdc: "0.001", txn_count: 21188494 },
    "7d": { avg_usdc: "0.50",     median_usdc: "0.10",  txn_count: 100 },
    "30d":{ avg_usdc: "0.40",     median_usdc: "0.001", txn_count: 500 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 21188494, breakeven_txn_count: 11334104, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0" },
    "7d": { txn_count: 100, breakeven_txn_count: 50, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0" },
    "30d":{ txn_count: 500, breakeven_txn_count: 250, gas_cents_per_dollar: "0.04", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0" },
  }},
  velocity: { windows: { all: { max_per_min: 100 }, "7d": { max_per_min: 80 }, "30d": { max_per_min: 90 } }, verified_daily: [] },
  claims: [],
  concentration: { windows: {
    all:  { payee: { total_entities: 69678 } },
    "7d": { payee: { total_entities: 1234 } },
    "30d":{ payee: { total_entities: 5678 } },
  }},
  excluded: { txn_count: 1578816, volume_usdc: "456380000.00" },
};

function overviewDom() {
  document.body.innerHTML = `
    <span id="ov-win"></span>
    <div id="ov-stats"></div>
    <div id="ov-bands"></div>
    <div id="ov-denom"></div>`;
}

function shapeDom() {
  document.body.innerHTML = `
    <span id="shp-win"></span>
    <div id="shp-big"></div>
    <div id="bands"></div>`;
}

function shellDom() {
  document.body.innerHTML = `<div id="shell"></div>`;
}

// ---------------------------------------------------------------------------
// Item 3.1 — rOverview shows median as TYPICAL PAYMENT
// ---------------------------------------------------------------------------
describe("item 3.1 — rOverview shows median as TYPICAL PAYMENT", () => {
  beforeEach(() => {
    overviewDom();
    setData(baseView);
    setWinLabel({ all: "all data Jan–Jun", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("rOverview contains the median value ($0.001) and TYPICAL PAYMENT (median) label", () => {
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    // median_usdc = "0.001" → fmtAmt("0.001") = "$0.001"
    expect(html).toContain("$0.001");
    expect(html).toContain("TYPICAL PAYMENT (median)");
  });

  it("rOverview does NOT label the bignum as avg", () => {
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    // The old label was "TYPICAL PAYMENT (avg)" — this must be gone
    expect(html).not.toContain("TYPICAL PAYMENT (avg)");
  });

  it("rOverview contains 'pulled up by large payments' text for the mean", () => {
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    expect(html).toContain("pulled up by large payments");
  });

  it("7d window uses the 7d median ($0.10 → fmtAmt strips trailing zero to $0.1)", () => {
    state.win = "7d";
    rOverview();
    const html = document.getElementById("ov-stats").innerHTML;
    // typical["7d"].median_usdc = "0.10" → fmtAmt("0.10") strips trailing zero → "$0.1"
    expect(html).toContain("$0.1");
    expect(html).toContain("TYPICAL PAYMENT (median)");
  });

  it("overview pinner context mentions 'median' and the median amount", () => {
    state.win = "all";
    const pin = PINNERS.overview();
    expect(pin.context).toContain("median");
    // $0.001 formatted
    expect(pin.context).toContain("$0.001");
  });
});

// ---------------------------------------------------------------------------
// Item 3.2 — rShape shows dual stacked bars (share of payments + share of dollars)
// ---------------------------------------------------------------------------
describe("item 3.2 — rShape shows dual stacked bars", () => {
  beforeEach(() => {
    shapeDom();
    setData(baseView);
    setWinLabel({ all: "all data Jan–Jun", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    state.bMetric = "tx";
    if (_clearPins) _clearPins();
  });

  it("rShape renders 'share of payments' label in #bands", () => {
    rShape();
    const html = document.getElementById("bands").innerHTML;
    expect(html).toContain("share of payments");
  });

  it("rShape renders 'share of dollars' label in #bands", () => {
    rShape();
    const html = document.getElementById("bands").innerHTML;
    expect(html).toContain("share of dollars");
  });

  it("dust segment in the payments bar has width > 50% (dust = 61.4% of payments in fixture)", () => {
    rShape();
    const html = document.getElementById("bands").innerHTML;
    // The dust <span class="seg" ...> in the payments bar should have width > 50%
    // dust = 13000000 / 21188494 = 61.36%
    // We parse the first .seg element's width style value
    const parser = new DOMParser();
    const doc = parser.parseFromString(html, "text/html");
    const segs = doc.querySelectorAll(".seg");
    // First segment in the first bbrow (payments bar) should be dust
    expect(segs.length).toBeGreaterThan(0);
    const firstSeg = segs[0];
    const widthStr = firstSeg.style.width;
    const widthVal = parseFloat(widthStr);
    expect(widthVal).toBeGreaterThan(50);
  });

  it("shape pinner context includes 'dust', 'payments', and '%'", () => {
    state.win = "all";
    const pin = PINNERS.shape();
    expect(pin.context).toContain("dust");
    expect(pin.context).toContain("payments");
    expect(pin.context).toContain("%");
  });
});

// ---------------------------------------------------------------------------
// Item 3.3 — checkIntegrity returns full check list (pass + fail)
// ---------------------------------------------------------------------------
describe("item 3.3 — checkIntegrity returns full check list", () => {
  it("returns objects with name, pass, and detail fields for every check", () => {
    const checks = checkIntegrity(baseView);
    expect(checks.length).toBeGreaterThan(0);
    const c = checks[0];
    expect(c).toHaveProperty("name");
    expect(c).toHaveProperty("pass");
    expect(c).toHaveProperty("detail");
  });

  it("all checks pass for a healthy fixture", () => {
    const checks = checkIntegrity(baseView);
    const failing = checks.filter(c => !c.pass);
    expect(failing.length).toBe(0);
  });

  it("count mismatch produces a failing check whose detail mentions both the band sum and the window total", () => {
    const brokenView = {
      ...baseView,
      windows: {
        ...baseView.windows,
        all: { ...baseView.windows.all, txn_count: 99999999 },
      },
    };
    const checks = checkIntegrity(brokenView);
    const failing = checks.filter(c => !c.pass);
    expect(failing.length).toBeGreaterThan(0);
    // The detail must mention the band sum (21188494) and the window total (99999999)
    const detail = failing[0].detail;
    expect(detail).toContain("21,188,494");
    expect(detail).toContain("99,999,999");
  });

  it("excluded-remainder check is included and detail mentions the total (21,188,494 + 1,578,816 = 22,767,310)", () => {
    const checks = checkIntegrity(baseView);
    const exclCheck = checks.find(c => c.name === "excluded-remainder");
    expect(exclCheck).toBeDefined();
    expect(exclCheck.pass).toBe(true);
    // detail should mention 22,767,310
    expect(exclCheck.detail).toContain("22,767,310");
  });
});

// ---------------------------------------------------------------------------
// Item 3.3 — rShell renders computed check results
// ---------------------------------------------------------------------------
describe("item 3.3 — rShell renders computed integrity checks", () => {
  beforeEach(() => {
    shellDom();
    setData(baseView);
    setWinLabel({ all: "all data Jan–Jun", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
  });

  it("renders a green ✓ glyph when all checks pass", () => {
    const checks = checkIntegrity(baseView);
    setIssues(checks);
    rShell();
    const html = document.getElementById("shell").innerHTML;
    expect(html).toContain('class="ok"');
    // Should NOT contain a fail glyph
    expect(html).not.toContain('class="fail"');
  });

  it("renders a class='fail' line when a check fails", () => {
    const brokenView = {
      ...baseView,
      windows: {
        ...baseView.windows,
        all: { ...baseView.windows.all, txn_count: 99999999 },
      },
    };
    const checks = checkIntegrity(brokenView);
    setIssues(checks);
    rShell();
    const html = document.getElementById("shell").innerHTML;
    expect(html).toContain('class="fail"');
  });

  it("shell output contains the formatted transaction count with commas (21,188,494)", () => {
    setIssues(checkIntegrity(baseView));
    rShell();
    const html = document.getElementById("shell").innerHTML;
    expect(html).toContain("21,188,494");
  });

  it("shell output mentions excluded count, verified count, and their total", () => {
    setIssues(checkIntegrity(baseView));
    rShell();
    // The excluded-remainder detail line should appear in shell output
    const html = document.getElementById("shell").innerHTML;
    // 22,767,310 is the total = verified + excluded
    expect(html).toContain("22,767,310");
  });

  it("shell renders the artifact scope from meta.scope when present", () => {
    const viewWithScope = { ...baseView, meta: { ...baseView.meta, scope: "verified-x402" } };
    setData(viewWithScope);
    setIssues(checkIntegrity(viewWithScope));
    rShell();
    const html = document.getElementById("shell").innerHTML;
    expect(html).toContain("verified-x402");
    expect(html).not.toContain("x402-attributed");
  });

  it("shell falls back to 'verified-x402' when meta.scope is absent", () => {
    // baseView.meta has no scope field — the fallback must fire
    setData(baseView);
    setIssues(checkIntegrity(baseView));
    rShell();
    const html = document.getElementById("shell").innerHTML;
    expect(html).toContain("verified-x402");
    expect(html).not.toContain("x402-attributed");
  });
});
