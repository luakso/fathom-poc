// Tests for item 7.1 - Pins that survive and behave.
// Covers: localStorage persistence keyed by generated_at, discard on key mismatch,
// deduplication (same panel+window replaces), window chip in tray list,
// and _clearPins() wiping localStorage.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";

const GA = "2026-07-01T00:00:00Z";
const LS_KEY = `fathom.pins.${GA}`;

function trayDom() {
  document.body.innerHTML = `
    <span id="pincount"></span>
    <aside id="tray"><div id="pinlist"></div>
      <textarea id="thread"></textarea><span id="charcount"></span>
      <canvas id="xcard" width="1200" height="675"></canvas>
      <button id="traytoggle"></button><button id="trayclose"></button>
      <button id="regen"></button><button id="copythread"></button>
      <button id="cardprev"></button><button id="cardnext"></button><button id="carddl"></button>
    </aside>`;
}

const mockData = {
  meta: { data_through_day: "2026-07-01", methodology_version: 1, generated_at: GA },
  windows: {
    all:  { txn_count: 5000, volume_usdc: "500.00", by_band: {} },
    "7d": { txn_count: 1000, volume_usdc: "100.00", by_band: {} },
    "30d":{ txn_count: 3000, volume_usdc: "300.00", by_band: {} },
  },
  daily: [["2026-06-30", 500, 50.00], ["2026-07-01", 400, 40.00]],
  monthly: [],
  typical: {
    all:  { avg_usdc: "0.10", median_usdc: "0.05", txn_count: 5000 },
    "7d": { avg_usdc: "0.10", median_usdc: "0.05", txn_count: 1000 },
    "30d":{ avg_usdc: "0.10", median_usdc: "0.05", txn_count: 3000 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 5000, breakeven_txn_count: 2500, gas_cents_per_dollar: "0.50" },
    "7d": { txn_count: 1000, breakeven_txn_count: 500,  gas_cents_per_dollar: "0.50" },
    "30d":{ txn_count: 3000, breakeven_txn_count: 1500, gas_cents_per_dollar: "0.50" },
  }},
  velocity: { windows: { all: { max_per_min: 50 }, "7d": { max_per_min: 30 }, "30d": { max_per_min: 40 } }, verified_daily: [] },
  claims: [],
  concentration: { windows: {
    all:  { payee: { total_entities: 1000 } },
    "7d": { payee: { total_entities: 200 } },
    "30d":{ payee: { total_entities: 600 } },
  }},
};

let addPin, clearPins, loadPins, rTray;

beforeEach(async () => {
  trayDom();
  setData(mockData);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
  state.dWin = "all";
  state.dMetric = "tx";
  const m = await import("../app/assets/js/tray.js");
  addPin    = m.addPin;
  clearPins = m._clearPins;
  loadPins  = m._loadPins;
  rTray     = m.rTray;
  clearPins(); // clears in-memory AND the matching localStorage key
});

// ---------------------------------------------------------------------------
// _clearPins wipes localStorage
// ---------------------------------------------------------------------------
describe("7.1 - _clearPins clears localStorage", () => {
  it("removes the localStorage entry for current generated_at", () => {
    localStorage.setItem(LS_KEY, JSON.stringify([{ key: "overview", win: "all", title: "T" }]));
    clearPins();
    expect(localStorage.getItem(LS_KEY)).toBeNull();
  });

  it("leaves entries for other keys untouched", () => {
    const otherKey = "fathom.pins.2025-01-01T00:00:00Z";
    localStorage.setItem(otherKey, "[]");
    clearPins();
    expect(localStorage.getItem(otherKey)).not.toBeNull();
    localStorage.removeItem(otherKey);
  });
});

// ---------------------------------------------------------------------------
// Persistence: _loadPins restores matching generated_at
// ---------------------------------------------------------------------------
describe("7.1 - _loadPins restores pins for matching generated_at", () => {
  it("loads persisted pins into the tray", () => {
    const pinData = [{ key: "overview", win: "all", title: "OVERVIEW · ALL", value: "$500", context: "ctx", denom: "d" }];
    localStorage.setItem(LS_KEY, JSON.stringify(pinData));
    loadPins();
    rTray();
    expect(document.getElementById("pincount").textContent).toBe("1");
  });

  it("discards pins from a different generated_at", () => {
    const oldKey = "fathom.pins.2025-01-01T00:00:00Z";
    localStorage.setItem(oldKey, JSON.stringify([{ key: "overview", win: "all", title: "OLD" }]));
    loadPins();
    rTray();
    expect(document.getElementById("pincount").textContent).toBe("0");
    localStorage.removeItem(oldKey);
  });

  it("addPin saves to localStorage", () => {
    addPin("overview");
    const raw = localStorage.getItem(LS_KEY);
    expect(raw).not.toBeNull();
    const saved = JSON.parse(raw);
    expect(Array.isArray(saved)).toBe(true);
    expect(saved.length).toBe(1);
    expect(saved[0].key).toBe("overview");
  });
});

// ---------------------------------------------------------------------------
// Deduplication: same panel+window replaces existing pin
// ---------------------------------------------------------------------------
describe("7.1 - addPin deduplication", () => {
  it("adding the same panel+window twice results in exactly one pin", () => {
    state.win = "all";
    addPin("overview");
    addPin("overview");
    expect(document.getElementById("pincount").textContent).toBe("1");
  });

  it("adding the same panel with a different window creates two pins", () => {
    state.win = "all";
    addPin("overview");
    state.win = "7d";
    addPin("overview");
    expect(document.getElementById("pincount").textContent).toBe("2");
  });

  it("duplicate pin does not grow localStorage", () => {
    state.win = "all";
    addPin("overview");
    addPin("overview");
    const saved = JSON.parse(localStorage.getItem(LS_KEY) || "[]");
    expect(saved.length).toBe(1);
  });
});

// ---------------------------------------------------------------------------
// Window chip visible in tray list
// ---------------------------------------------------------------------------
describe("7.1 - window chip in tray list", () => {
  it("pinitem shows the window name when win is '7d'", () => {
    state.win = "7d";
    addPin("overview");
    const html = document.getElementById("pinlist").innerHTML;
    expect(html).toContain("7d");
  });

  it("pinitem shows 'all' when win is 'all'", () => {
    state.win = "all";
    addPin("overview");
    const html = document.getElementById("pinlist").innerHTML;
    expect(html).toContain("all");
  });
});
