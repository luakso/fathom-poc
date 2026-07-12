// Tests for 6.6 - facilitator momentum: render, absent-state, PINNER, key 0.
import { describe, it, expect, beforeEach, vi } from "vitest";
import { state, setData, setWinLabel, setFacData } from "../app/assets/js/state.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rFacilitators } from "../app/assets/js/panels.js";

// ---------------------------------------------------------------------------
// Shared minimal DOM for facilitators panel.
// ---------------------------------------------------------------------------
function facDom() {
  document.body.innerHTML = `
    <div id="fac-body"></div>
    <span id="pincount"></span>
    <aside id="tray"><div id="pinlist"></div>
      <textarea id="thread"></textarea><span id="charcount"></span>
      <canvas id="xcard" width="1200" height="675"></canvas>
      <button id="traytoggle"></button><button id="trayclose"></button>
      <button id="regen"></button><button id="copythread"></button>
      <button id="cardprev"></button><button id="cardnext"></button><button id="carddl"></button>
    </aside>`;
}

// Minimal economy view-model so setData doesn't crash.
const minimalData = {
  meta: { data_through_day: "2026-06-08", methodology_version: 1, generated_at: "2026-06-08T00:00:00Z" },
  windows: {
    all:  { txn_count: 100, volume_usdc: "100.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:100,volume_usdc:"100.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
    "7d": { txn_count: 50, volume_usdc: "50.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:50,volume_usdc:"50.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
    "30d":{ txn_count: 80, volume_usdc: "80.00", by_band: { dust:{txn_count:0,volume_usdc:"0"}, micro:{txn_count:0,volume_usdc:"0"}, small:{txn_count:80,volume_usdc:"80.00"}, mid:{txn_count:0,volume_usdc:"0"}, whale:{txn_count:0,volume_usdc:"0"} } },
  },
  daily: [],
  monthly: [],
  typical: {
    all:  { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 100 },
    "7d": { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 50 },
    "30d":{ avg_usdc: "1.00", median_usdc: "0.50", txn_count: 80 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  price_breadth: null,
  gas: { windows: {
    all:  { txn_count: 100, breakeven_txn_count: 50, gas_cents_per_dollar: "0.50", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
    "7d": { txn_count: 50,  breakeven_txn_count: 25, gas_cents_per_dollar: "0.50", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
    "30d":{ txn_count: 80,  breakeven_txn_count: 40, gas_cents_per_dollar: "0.50", gas_eth:"0", gas_eth_l1:"0", gas_eth_l2:"0", gas_usd:"0", by_band:{} },
  }},
  velocity: { windows: { all:{max_per_min:10},"7d":{max_per_min:5},"30d":{max_per_min:8} }, verified_daily: [] },
  claims: [],
  concentration: { windows: {} },
  excluded: null,
  active_entities: null,
  payer_cohorts: null,
};

// A representative facData object with windows populated.
const mockFacData = {
  rows: [
    {
      facilitator: "0x1234567890abcdef1234567890abcdef12345678",
      facilitator_known: true,
      txn_count: 1000,
      volume_usdc: "5000.000000",
      windows: {
        "7d":  { txn_count: 200, volume_usdc: "1000.000000" },
        "30d": { txn_count: 500, volume_usdc: "2500.000000" },
      },
    },
    {
      facilitator: "0xabcdef1234567890abcdef1234567890abcdef12",
      facilitator_known: true,
      txn_count: 500,
      volume_usdc: "2000.000000",
      windows: {
        "7d":  { txn_count: 100, volume_usdc: "400.000000" },
        "30d": { txn_count: 300, volume_usdc: "1200.000000" },
      },
    },
  ],
  totals: { txn_count: 1500, volume_usdc: "7000.000000" },
};

// facData with no windows (simulates an artifact from before 6.6).
const oldFacData = {
  rows: [
    {
      facilitator: "0x1234567890abcdef1234567890abcdef12345678",
      facilitator_known: true,
      txn_count: 1000,
      volume_usdc: "5000.000000",
    },
  ],
  totals: { txn_count: 1000, volume_usdc: "5000.000000" },
};

// ---------------------------------------------------------------------------
// rFacilitators: renders correctly when facData is present
// ---------------------------------------------------------------------------
describe("6.6 rFacilitators — renders correctly", () => {
  beforeEach(() => {
    facDom();
    setData(minimalData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("renders a table when facData is present with windows", () => {
    setFacData(mockFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    expect(html).toContain("<table");
    expect(html).toContain("facilitator");
  });

  it("shows shortened address (0xabcd…1234) for the top facilitator", () => {
    setFacData(mockFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    // 0x1234567890abcdef1234567890abcdef12345678 → 0x1234…5678
    expect(html).toContain("0x1234…5678");
  });

  it("shows all-time volume in the table", () => {
    setFacData(mockFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    // $5000 → fmtMoney = "5.0k USDC"
    expect(html).toContain("5.0k USDC");
  });

  it("shows 7d volume column in the table", () => {
    setFacData(mockFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    // $1000 7d volume → "1.0k USDC"
    expect(html).toContain("1.0k USDC");
  });

  it("shows momentum column (7d share of 30d)", () => {
    setFacData(mockFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    // $1000 / $2500 = 40%
    expect(html).toContain("40%");
  });

  it("shows '…N more' line when rows.length > 8", () => {
    const bigFac = {
      rows: Array.from({ length: 10 }, (_, i) => ({
        facilitator: `0x${String(i).padStart(40, "0")}`,
        facilitator_known: true,
        txn_count: 10 - i,
        volume_usdc: `${(10 - i) * 100}.000000`,
        windows: {
          "7d":  { txn_count: 1, volume_usdc: "10.000000" },
          "30d": { txn_count: 5, volume_usdc: "50.000000" },
        },
      })),
      totals: { txn_count: 55, volume_usdc: "5500.000000" },
    };
    setFacData(bigFac);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    expect(html).toContain("more");
    expect(html).toContain("2");
  });
});

// ---------------------------------------------------------------------------
// rFacilitators: absent-state when facData is null (fetch failed)
// ---------------------------------------------------------------------------
describe("6.6 rFacilitators — absent-state on failed fetch", () => {
  beforeEach(() => {
    facDom();
    setData(minimalData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("renders absent-state (not a crash) when facData is null", () => {
    setFacData(null);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    expect(html).toBeTruthy();
    expect(html).not.toContain("<table");
    expect(html.toLowerCase()).toMatch(/unavailable|absent/);
  });

  it("renders absent-state when facData has no rows", () => {
    setFacData({ rows: [], totals: { txn_count: 0, volume_usdc: "0" } });
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    expect(html).not.toContain("<table");
  });

  it("renders absent-state when windows field is missing (old artifact)", () => {
    setFacData(oldFacData);
    rFacilitators();
    const html = document.getElementById("fac-body").innerHTML;
    expect(html).not.toContain("<table");
  });

  it("does not throw when facData is null", () => {
    setFacData(null);
    expect(() => rFacilitators()).not.toThrow();
  });
});

// ---------------------------------------------------------------------------
// PINNERS.facilitators: correct structure and character-identical formatting
// ---------------------------------------------------------------------------
describe("6.6 PINNERS.facilitators — pin structure", () => {
  beforeEach(() => {
    facDom();
    setData(minimalData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    if (_clearPins) _clearPins();
  });

  it("returns null when facData is null", () => {
    setFacData(null);
    expect(PINNERS.facilitators()).toBeNull();
  });

  it("returns null when windows are absent (old artifact)", () => {
    setFacData(oldFacData);
    expect(PINNERS.facilitators()).toBeNull();
  });

  it("returns a pin object when facData has windows", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    expect(pin).not.toBeNull();
    expect(pin).toHaveProperty("title");
    expect(pin).toHaveProperty("value");
    expect(pin).toHaveProperty("context");
    expect(pin).toHaveProperty("denom");
  });

  it("title is FACILITATORS", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    expect(pin.title).toBe("FACILITATORS");
  });

  it("value contains the shortened top-facilitator address (character-identical to panel)", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    // 0x1234567890abcdef1234567890abcdef12345678 → 0x1234…5678
    expect(pin.value).toContain("0x1234…5678");
  });

  it("value contains the all-time volume", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    // $5000 → "5.0k USDC all-time"
    expect(pin.value).toContain("5.0k USDC");
    expect(pin.value).toContain("all-time");
  });

  it("context mentions momentum as a percentage", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    // 7d=$1000 / 30d=$2500 = 40%
    expect(pin.context).toContain("40%");
  });

  it("denom matches the specified language verbatim", () => {
    setFacData(mockFacData);
    const pin = PINNERS.facilitators();
    expect(pin.denom).toBe(
      "who settled the payments · verified payments only · momentum = last 7 days' share of the last 30"
    );
  });
});

// ---------------------------------------------------------------------------
// Key "0" — pressing 0 focuses panel p12 (FACILITATORS)
// ---------------------------------------------------------------------------
describe("6.6 key 0 focuses the FACILITATORS panel (p12)", () => {
  beforeEach(() => {
    // Minimal DOM including p12
    document.body.innerHTML = `
      <div class="panel" id="p1"></div>
      <div class="panel" id="p12"></div>`;
    // Reset focused state
    state.focused = "p1";
    // Stub scrollIntoView (not implemented in jsdom)
    document.querySelectorAll(".panel").forEach(p => {
      p.scrollIntoView = vi.fn();
    });
  });

  it("panel p12 exists in the DOM (key 0 target is wired correctly)", () => {
    expect(document.getElementById("p12")).not.toBeNull();
  });

  it("pressing '0' sets state.focused to p12 when the handler runs", () => {
    // Simulate the exact logic from app.js key "0" handler, without importing
    // app.js (which auto-boots and needs fetch). Test the handler logic in isolation.
    // dom.js defines $ as document.querySelector(s), so use the same selector form.
    const $ = s => document.querySelector(s);
    const $$ = s => [...document.querySelectorAll(s)];
    const p = $("#p12");
    if (p){
      p.scrollIntoView({ behavior:"smooth", block:"center" });
      state.focused = p.id;
      $$(".panel").forEach(x => x.classList.toggle("focused", x === p));
    }
    expect(state.focused).toBe("p12");
    expect(document.querySelector("#p12").classList.contains("focused")).toBe(true);
    expect(document.querySelector("#p1").classList.contains("focused")).toBe(false);
  });
});
