// Tests for item 4.5 — loading indicator present at boot, cleared after render/fatal.
// vi.resetModules() in beforeEach ensures app.js re-boots each test.
import { describe, it, expect, beforeEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here     = dirname(fileURLToPath(import.meta.url));
const htmlPath = resolve(here, "../app/index.html");

// Minimal economy.json payload that passes reshape and all gates
const minimalEconomy = {
  methodology_version: 1,
  generated_at: "2026-06-18T00:00:00Z",
  data_through_day: "2026-06-18",
  data: {
    windows: {
      all:  { txn_count: 10, volume_usdc: "5.00", by_band: {
        dust:  { txn_count: 2, volume_usdc: "0.02" },
        micro: { txn_count: 2, volume_usdc: "0.20" },
        small: { txn_count: 2, volume_usdc: "2.00" },
        mid:   { txn_count: 2, volume_usdc: "1.00" },
        whale: { txn_count: 2, volume_usdc: "1.78" },
      }},
      "7d": { txn_count: 3, volume_usdc: "1.50", by_band: {
        dust:  { txn_count: 1, volume_usdc: "0.01" },
        micro: { txn_count: 1, volume_usdc: "0.10" },
        small: { txn_count: 1, volume_usdc: "1.00" },
        mid:   { txn_count: 0, volume_usdc: "0.00" },
        whale: { txn_count: 0, volume_usdc: "0.39" },
      }},
      "30d":{ txn_count: 7, volume_usdc: "3.50", by_band: {
        dust:  { txn_count: 1, volume_usdc: "0.01" },
        micro: { txn_count: 2, volume_usdc: "0.20" },
        small: { txn_count: 2, volume_usdc: "2.00" },
        mid:   { txn_count: 1, volume_usdc: "0.50" },
        whale: { txn_count: 1, volume_usdc: "0.79" },
      }},
    },
    daily_series: [{ day: "2026-06-18", txn_count: 10, volume_usdc: "5.00" }],
    monthly_series: [],
    typical_payment: {
      all:  { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 10 },
      "7d": { avg_usdc: "0.50", median_usdc: "0.10", txn_count: 3 },
      "30d":{ avg_usdc: "0.50", median_usdc: "0.10", txn_count: 7 },
    },
    price_points: { all: [], "7d": [], "30d": [] },
    gas: {
      windows: {
        all:  { txn_count: 10, breakeven_txn_count: 5, gas_eth: "0.001", gas_eth_l1: "0", gas_eth_l2: "0.001", gas_usd: "2.50", gas_cents_per_dollar: "50.00" },
        "7d": { txn_count: 3,  breakeven_txn_count: 2, gas_eth: "0.001", gas_eth_l1: "0", gas_eth_l2: "0.001", gas_usd: "0.75", gas_cents_per_dollar: "50.00" },
        "30d":{ txn_count: 7,  breakeven_txn_count: 4, gas_eth: "0.001", gas_eth_l1: "0", gas_eth_l2: "0.001", gas_usd: "1.75", gas_cents_per_dollar: "50.00" },
      },
    },
    velocity: {
      windows: { all: { max_per_min: 5 }, "7d": { max_per_min: 3 }, "30d": { max_per_min: 4 } },
      daily_series: [],
    },
    claims: [],
    concentration: { windows: {} },
  },
};

async function settle(selector, ms = 2000) {
  const deadline = Date.now() + ms;
  while (!document.querySelector(selector) && Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 0));
  }
}

describe("4.5 - loading state indicator", () => {
  beforeEach(() => {
    // Reset module cache so app.js re-runs boot() in every test
    vi.resetModules();
    const html = readFileSync(htmlPath, "utf8");
    const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\S]*/, "");
    document.body.innerHTML = body;
    vi.stubGlobal("localStorage", { getItem: () => "1", setItem: () => {} });
  });

  it("loading text is visible in #st-cons before fetch resolves", async () => {
    // Hold the fetch open so we can inspect the DOM during the await
    let resolveFetch;
    vi.stubGlobal("fetch", () => new Promise(res => { resolveFetch = res; }));

    // Import triggers boot() which sets loading text then suspends at loadEconomy()
    await import("../app/assets/js/app.js");

    // fetch is still pending — loading text must be set
    const stCons = document.getElementById("st-cons");
    expect(stCons).not.toBeNull();
    expect(stCons.textContent).toContain("loading");

    // resolve fetch with a 404 so the pending boot can finish (clean up)
    resolveFetch({ ok: false, status: 404 });
    await new Promise(r => setTimeout(r, 50));
  });

  it("loading text is cleared after successful render", async () => {
    vi.stubGlobal("fetch", () => Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(minimalEconomy),
    }));

    await import("../app/assets/js/app.js");
    // Wait for applyMeta to stamp #st-through
    await settle("#st-through");
    await new Promise(r => setTimeout(r, 50));

    const stCons = document.getElementById("st-cons");
    // applyMeta sets st-cons to "conservation ✓" or "conservation ✗"
    expect(stCons.textContent).not.toContain("loading");
    expect(stCons.textContent).toMatch(/conservation/i);
  });

  it("loading text is cleared after a fatal fetch error", async () => {
    vi.stubGlobal("fetch", () => Promise.resolve({ ok: false, status: 404 }));

    await import("../app/assets/js/app.js");
    // Wait for the fatal screen
    await settle("#fatal.open");
    await new Promise(r => setTimeout(r, 50));

    const stCons = document.getElementById("st-cons");
    expect(stCons.textContent).not.toContain("loading");
  });
});
