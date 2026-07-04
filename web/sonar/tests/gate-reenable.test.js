// Regression test for: gate "continue anyway" must work after a re-enable via help overlay.
// The boot in app.js registered a { once: true } listener which was consumed after the
// first dismiss — making "continue" dead after a re-enable + resize without a full reload.
//
// Strategy: import real app.js (using vi.resetModules so boot re-runs), mock fetch with a
// minimal economy payload, narrow innerWidth so the gate opens at boot, then exercise the
// full round-trip: open → continue → re-enable → reopen → continue again.
import { describe, it, expect, beforeEach, vi } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here     = dirname(fileURLToPath(import.meta.url));
const htmlPath = resolve(here, "../app/index.html");
const GATE_KEY = "fathom.smallScreenOk";

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
    concentration: { windows: { all: { payee: { total_entities: 42 } } } },
  },
};

// Wait until a selector exists in the DOM (polls microtask queue).
async function settle(selector, ms = 2000) {
  const deadline = Date.now() + ms;
  while (!document.querySelector(selector) && Date.now() < deadline) {
    await new Promise(r => setTimeout(r, 0));
  }
}

beforeEach(() => {
  vi.resetModules();
  localStorage.removeItem(GATE_KEY);
  const html = readFileSync(htmlPath, "utf8");
  const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\W]*/, "");
  document.body.innerHTML = body;
  // Narrow viewport so gate activates at boot.
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 500 });
  vi.stubGlobal("fetch", () => Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve(minimalEconomy),
  }));
});

describe("7.4 - gate continue works after re-enable via help overlay (regression)", () => {
  it("continue works on second gate open after re-enable clears the opt-out flag", async () => {
    // Boot app.js — narrow screen means maybeGate opens the gate.
    await import("../app/assets/js/app.js");
    await settle("#gate.open");

    const gate = document.getElementById("gate");
    const btn  = document.getElementById("gate-continue");

    // Step 1: gate is open.
    expect(gate.classList.contains("open")).toBe(true);

    // Step 2: first "continue anyway" - dismisses gate and sets opt-out.
    btn.click();
    expect(localStorage.getItem(GATE_KEY)).toBe("1");
    expect(gate.classList.contains("open")).toBe(false);

    // Step 3: user re-enables via help overlay (clears opt-out flag).
    localStorage.removeItem(GATE_KEY);

    // Step 4: simulate what maybeGate does on a narrow resize — gate reopens.
    // (We bypass the debounced resize listener and manipulate the class directly,
    // because we're testing the continue listener, not maybeGate itself.)
    gate.classList.add("open");
    expect(gate.classList.contains("open")).toBe(true);

    // Step 5 (regression): "continue" must work again.
    btn.click();
    expect(localStorage.getItem(GATE_KEY)).toBe("1");
    expect(gate.classList.contains("open")).toBe(false);
  });
});
