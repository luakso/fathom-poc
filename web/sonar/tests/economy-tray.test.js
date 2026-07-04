// Tests for economy tray pin generators (items 1.1, 1.2, 1.3, 1.6).
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";
import { createTray } from "../app/assets/js/lib/report-tray.js";

// ---------------------------------------------------------------------------
// Shared minimal DOM for tray tests.
// ---------------------------------------------------------------------------
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

// ---------------------------------------------------------------------------
// Shared minimal dataset.
// tx-peak: day 0 ("2026-05-24", tx=1000, usd=10)
// usd-peak: day 9 ("2026-06-02", tx=100, usd=200)
// These differ so item-1.2 tests can distinguish which column is active.
// ---------------------------------------------------------------------------
const mockDaily = [
  ["2026-05-24", 1000,  10.00],
  ["2026-05-25",  900,  20.00],
  ["2026-05-26",  800,  30.00],
  ["2026-05-27",  700,  40.00],
  ["2026-05-28",  600,  50.00],
  ["2026-05-29",  500,  60.00],
  ["2026-05-30",  400,  70.00],
  ["2026-05-31",  300,  80.00],
  ["2026-06-01",  200,  90.00],
  ["2026-06-02",  100, 200.00],
];

const mockData = {
  meta: { data_through_day: "2026-06-02", methodology_version: 1 },
  windows: {
    all:  { txn_count: 5500, volume_usdc: "650.00", by_band: {} },
    "7d": { txn_count: 2100, volume_usdc: "590.00", by_band: {} },
    "30d":{ txn_count: 5500, volume_usdc: "650.00", by_band: {} },
  },
  daily: mockDaily,
  monthly: [],
  typical: {
    all:  { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 5500 },
    "7d": { avg_usdc: "1.00", median_usdc: "0.50", txn_count: 2100 },
    "30d":{ avg_usdc: "1.00", median_usdc: "0.50", txn_count: 5500 },
  },
  price_points: {
    all:  [{ amount_usdc: "0.001", txn_count: 374, txn_share_pct: "37.42", payee_count: 500 }],
    "7d": [],
    "30d":[],
  },
  gas: { windows: {
    all:  { txn_count: 5500, breakeven_txn_count: 2750, gas_cents_per_dollar: "0.50" },
    "7d": { txn_count: 2100, breakeven_txn_count: 1000, gas_cents_per_dollar: "0.50" },
    "30d":{ txn_count: 5500, breakeven_txn_count: 2750, gas_cents_per_dollar: "0.50" },
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

const mockMeta = { data_through_day: "2026-06-02", methodology_version: 1 };

// ---------------------------------------------------------------------------
// Item 1.1 — footer text in lib/report-tray.js
// ---------------------------------------------------------------------------
describe("item 1.1 — lib/report-tray.js footer text", () => {
  beforeEach(() => { trayDom(); });

  it("genThread must not contain 'no row dropped'", () => {
    const t = createTray({ brand: "economy", meta: mockMeta });
    t.init();
    t.addPin({ title: "T", value: "$1.00", context: "ctx", denom: "d" });
    t.genThread();
    expect(document.getElementById("thread").value).not.toContain("no row dropped");
  });

  it("genThread footer says 'verified payments only. unverified transfers excluded. denominators stated.'", () => {
    const t = createTray({ brand: "economy", meta: mockMeta });
    t.init();
    t.addPin({ title: "T", value: "$1.00", context: "ctx", denom: "d" });
    t.genThread();
    expect(document.getElementById("thread").value)
      .toContain("verified payments only. unverified transfers excluded. denominators stated.");
  });
});

// ---------------------------------------------------------------------------
// Item 1.1 — footer text in economy tray.js (via genThread)
// ---------------------------------------------------------------------------
describe("item 1.1 — economy tray.js genThread footer text", () => {
  let addPinFn, genThreadFn, clearPinsFn;

  beforeEach(async () => {
    trayDom();
    setData(mockData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    state.dWin = "30d";
    state.dMetric = "tx";
    const m = await import("../app/assets/js/tray.js");
    addPinFn    = m.addPin;
    genThreadFn = m.genThread;
    clearPinsFn = m._clearPins;
    clearPinsFn();
  });

  it("genThread must not contain 'no row dropped'", () => {
    addPinFn("overview");
    genThreadFn();
    expect(document.getElementById("thread").value).not.toContain("no row dropped");
  });

  it("genThread footer says 'verified payments only. unverified transfers excluded. denominators stated.'", () => {
    addPinFn("overview");
    genThreadFn();
    expect(document.getElementById("thread").value)
      .toContain("verified payments only. unverified transfers excluded. denominators stated.");
  });
});

// ---------------------------------------------------------------------------
// Item 1.2 — daily pinner peak follows the active metric
// ---------------------------------------------------------------------------
describe("item 1.2 — daily pinner peak follows active metric", () => {
  let PINNERS, clearPins;

  beforeEach(async () => {
    setData(mockData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    state.dWin = "all";   // full series so both peaks are in view
    const m = await import("../app/assets/js/tray.js");
    PINNERS   = m.PINNERS;
    clearPins = m._clearPins;
    clearPins();
  });

  it("tx mode: headline contains the tx-peak count and its day", () => {
    state.dMetric = "tx";
    const pin = PINNERS.daily();
    // tx-peak is day 0 ("2026-05-24", tx=1000)
    expect(pin.value).toContain("1,000");
    expect(pin.context).toContain("2026-05-24");
  });

  it("usd mode: headline is money-formatted and peak day comes from the USD column", () => {
    state.dMetric = "usd";
    const pin = PINNERS.daily();
    // usd-peak is day 9 ("2026-06-02", usd=200)
    expect(pin.value).toContain("$");          // money format
    expect(pin.value).not.toContain("tx/day"); // not the tx label
    expect(pin.context).toContain("2026-06-02");
  });

  it("usd mode: peak day is not the tx-peak day", () => {
    state.dMetric = "usd";
    const pin = PINNERS.daily();
    // if erroneously using tx column, day would be "2026-05-24"
    expect(pin.context).not.toContain("2026-05-24");
  });
});

// ---------------------------------------------------------------------------
// Item 1.3 — overview pinner series: sliced window + volume column
// ---------------------------------------------------------------------------
describe("item 1.3 — overview pinner series: sliced window and volume column", () => {
  let PINNERS, clearPins;

  beforeEach(async () => {
    setData(mockData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    const m = await import("../app/assets/js/tray.js");
    PINNERS   = m.PINNERS;
    clearPins = m._clearPins;
    clearPins();
  });

  it("7d window: series has 7 points from the trailing volume column", () => {
    state.win = "7d";
    const pin = PINNERS.overview();
    // trailing 7 from 10-row series → indices 3-9
    expect(pin.series.length).toBe(7);
    // values from volume column (index 2), not tx column (index 1)
    expect(pin.series[0]).toBe(mockDaily[3][2]); // 40.00
    expect(pin.series[6]).toBe(mockDaily[9][2]); // 200.00
  });

  it("all window: series covers all rows using the volume column", () => {
    state.win = "all";
    const pin = PINNERS.overview();
    expect(pin.series.length).toBe(mockDaily.length);
    // first value must be volume (10.00), not tx count (1000)
    expect(pin.series[0]).toBe(mockDaily[0][2]);
    expect(pin.series[0]).not.toBe(mockDaily[0][1]);
  });
});

// ---------------------------------------------------------------------------
// Item 1.6 — price-point pin share uses one-decimal formatting (matching panel)
// ---------------------------------------------------------------------------
describe("item 1.6 — price-point pin share is one-decimal, matching the panel", () => {
  let PINNERS, clearPins;

  beforeEach(async () => {
    setData(mockData);
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.win = "all";
    const m = await import("../app/assets/js/tray.js");
    PINNERS   = m.PINNERS;
    clearPins = m._clearPins;
    clearPins();
  });

  it("pin context share string uses one decimal (matches panel rendering)", () => {
    const pin = PINNERS.price();
    // raw txn_share_pct = "37.42"; panel uses .toFixed(1) = "37.4%"
    expect(pin.context).toContain("37.4%");
    expect(pin.context).not.toContain("37.42%");
  });
});
