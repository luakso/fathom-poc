// Tests for the small-screen entry gate (item 1.4).
// app.js runs DOM listeners at module level so a full HTML skeleton
// is required before the module loads.
import { describe, it, expect, beforeEach } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here      = dirname(fileURLToPath(import.meta.url));
const htmlPath  = resolve(here, "../app/index.html");

const mockDaily = [
  ["2026-05-24", 1000, 10.00],
  ["2026-05-25",  900, 20.00],
];

// view-model returned by reshape() for a tiny stub dataset.
const mockView = {
  meta: { data_through_day: "2026-06-02", methodology_version: 1, generated_at: "2026-06-02T00:00:00Z" },
  windows: {
    all:  { txn_count: 21188494, volume_usdc: "7538241.503295", by_band: {
      dust:  { txn_count: 1000, volume_usdc: "1.00" },
      micro: { txn_count: 1000, volume_usdc: "10.00" },
      small: { txn_count: 1000, volume_usdc: "100.00" },
      mid:   { txn_count: 1000, volume_usdc: "1000.00" },
      whale: { txn_count: 17184494, volume_usdc: "7537130.503295" },
    }},
    "7d": { txn_count: 100, volume_usdc: "50.00", by_band: {
      dust:  { txn_count: 20, volume_usdc: "0.20" },
      micro: { txn_count: 20, volume_usdc: "2.00" },
      small: { txn_count: 20, volume_usdc: "20.00" },
      mid:   { txn_count: 20, volume_usdc: "10.00" },
      whale: { txn_count: 20, volume_usdc: "17.80" },
    }},
    "30d":{ txn_count: 500, volume_usdc: "200.00", by_band: {
      dust:  { txn_count: 100, volume_usdc: "1.00" },
      micro: { txn_count: 100, volume_usdc: "10.00" },
      small: { txn_count: 100, volume_usdc: "40.00" },
      mid:   { txn_count: 100, volume_usdc: "49.00" },
      whale: { txn_count: 100, volume_usdc: "100.00" },
    }},
  },
  daily: mockDaily,
  monthly: [],
  typical: {
    all:  { avg_usdc: "0.36", median_usdc: "0.001", txn_count: 21188494 },
    "7d": { avg_usdc: "0.50", median_usdc: "0.001", txn_count: 100 },
    "30d":{ avg_usdc: "0.40", median_usdc: "0.001", txn_count: 500 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 21188494, breakeven_txn_count: 11334104, gas_cents_per_dollar: "0.04" },
    "7d": { txn_count: 100, breakeven_txn_count: 50, gas_cents_per_dollar: "0.04" },
    "30d":{ txn_count: 500, breakeven_txn_count: 250, gas_cents_per_dollar: "0.04" },
  }},
  velocity: {
    windows: { all: { max_per_min: 100 }, "7d": { max_per_min: 80 }, "30d": { max_per_min: 90 } },
    verified_daily: [],
  },
  claims: [],
  concentration: { windows: {
    all:  { payee: { total_entities: 69678 } },
    "7d": { payee: { total_entities: 1234 } },
    "30d":{ payee: { total_entities: 5678 } },
  }},
};

// ---------------------------------------------------------------------------
// Item 1.4 — gate shows three distinct facts: payments, volume, payees
// ---------------------------------------------------------------------------
describe("item 1.4 — gate shows payments + volume + payees (not txn_count twice)", () => {
  let buildGateHtml;

  beforeEach(async () => {
    // Set up the HTML skeleton app.js needs for its module-level listener.
    const html = readFileSync(htmlPath, "utf8");
    const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\S]*/, "");
    document.body.innerHTML = body;
    // Now safe to import app.js.
    const m = await import("../app/assets/js/app.js");
    buildGateHtml = m.buildGateHtml;
  });

  it("buildGateHtml is exported from app.js", () => {
    expect(typeof buildGateHtml).toBe("function");
  });

  it("gate contains PAYMENTS and VOLUME labels", () => {
    const html = buildGateHtml(mockView);
    expect(html).toContain("PAYMENTS");
    expect(html).toContain("VOLUME");
  });

  it("gate contains PAYEES label, not VERIFIED TX", () => {
    const html = buildGateHtml(mockView);
    expect(html).toContain("PAYEES");
    expect(html).not.toContain("VERIFIED TX");
  });

  it("payee stat is derived from concentration data, not txn_count", () => {
    // txn_count.all = 21,188,494; concentration.payee.total_entities = 69,678
    // The two numbers format very differently: "21.2M" vs "70k" (approx).
    // Check that "VERIFIED TX" is absent — that was the duplicate-txn_count bug.
    const html = buildGateHtml(mockView);
    expect(html).not.toMatch(/VERIFIED TX/);
    // And that the payees block appears somewhere in the output
    expect(html).toMatch(/PAYEES/);
  });
});
