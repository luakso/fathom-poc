// Tests for item 7.2 - Auto-compose the thread.
// Covers: opening tray with pins auto-composes; hand-edits are never clobbered;
// REGEN always recomposes and clears the dirty flag.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";

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
  meta: { data_through_day: "2026-07-01", methodology_version: 1, generated_at: "2026-07-01T00:00:00Z" },
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

let addPin, toggleTray, genThread, initTray, clearPins;

beforeEach(async () => {
  trayDom();
  setData(mockData);
  setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
  state.win = "all";
  state.dWin = "all";
  state.dMetric = "tx";
  const m = await import("../app/assets/js/tray.js");
  addPin     = m.addPin;
  toggleTray = m.toggleTray;
  genThread  = m.genThread;
  initTray   = m.initTray;
  clearPins  = m._clearPins;
  clearPins();
  initTray(); // wire button listeners (REGEN, textarea input, etc.)
  // Ensure tray starts closed
  document.getElementById("tray").classList.remove("open");
  // Ensure thread is empty
  document.getElementById("thread").value = "";
});

// ---------------------------------------------------------------------------
// Auto-compose when opening with pins
// ---------------------------------------------------------------------------
describe("7.2 - auto-compose on tray open", () => {
  it("opening tray with pins fills the thread textarea", () => {
    addPin("overview");
    toggleTray(true);
    const text = document.getElementById("thread").value;
    expect(text.length).toBeGreaterThan(0);
    expect(text).toContain("verified payments only");
  });

  it("opening tray with no pins leaves the thread empty", () => {
    toggleTray(true);
    expect(document.getElementById("thread").value).toBe("");
  });

  it("toggling tray closed then open with pins composes again", () => {
    addPin("overview");
    toggleTray(true);
    const first = document.getElementById("thread").value;
    toggleTray(false);
    document.getElementById("thread").value = "";
    toggleTray(true);
    expect(document.getElementById("thread").value).toBe(first);
  });
});

// ---------------------------------------------------------------------------
// Hand-edits are never clobbered
// ---------------------------------------------------------------------------
describe("7.2 - dirty flag prevents clobbering hand-edits", () => {
  it("hand-editing the textarea prevents auto-compose on next open", () => {
    addPin("overview");
    toggleTray(true);
    // Simulate a hand-edit
    const ta = document.getElementById("thread");
    ta.value = "my custom text";
    ta.dispatchEvent(new Event("input"));
    // Close and reopen
    toggleTray(false);
    toggleTray(true);
    // The custom text must survive
    expect(document.getElementById("thread").value).toBe("my custom text");
  });
});

// ---------------------------------------------------------------------------
// REGEN recomposes and clears the dirty flag
// ---------------------------------------------------------------------------
describe("7.2 - REGEN clears dirty and recomposes", () => {
  it("clicking REGEN replaces hand-edits with composed thread", () => {
    addPin("overview");
    toggleTray(true);
    // Dirty it
    const ta = document.getElementById("thread");
    ta.value = "custom";
    ta.dispatchEvent(new Event("input"));
    // Click REGEN
    document.getElementById("regen").click();
    expect(document.getElementById("thread").value).toContain("fathom");
    expect(document.getElementById("thread").value).not.toBe("custom");
  });

  it("after REGEN, re-opening tray still shows composed content (dirty cleared)", () => {
    addPin("overview");
    toggleTray(true);
    const ta = document.getElementById("thread");
    ta.value = "custom";
    ta.dispatchEvent(new Event("input"));
    document.getElementById("regen").click();
    const afterRegen = ta.value;
    toggleTray(false);
    toggleTray(true);
    // Must still be composed, not fall back to custom
    expect(document.getElementById("thread").value).toBe(afterRegen);
    expect(document.getElementById("thread").value).toContain("fathom");
  });
});
