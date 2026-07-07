// Item 8.3 — Pinner equivalence tests.
// For every PINNERS entry, assert the exported value/context strings carry the
// same formatted key numbers as what the corresponding panel renders from the
// same fixture. This catches the class of bug that already hit twice: daily
// peak metric and percentile strip separators.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel, setFacData } from "../app/assets/js/state.js";
import { PINNERS, _clearPins } from "../app/assets/js/tray.js";
import { rOverview, rShape, rGas, rClaims } from "../app/assets/js/panels.js";
import { fmtUSDCAmt, fmtUSDC, fmtInt, fmtCount, claimVerdict } from "../app/assets/js/format.js";

// ---------------------------------------------------------------------------
// Shared fixture: conservation passes so no banner.
// ---------------------------------------------------------------------------
const FIX = {
  meta: {
    methodology_version: 1,
    data_through_day: "2026-06-18",
    generated_at: "2026-06-18T00:00:00Z",
    scope: "verified-x402",
  },
  windows: {
    all: {
      txn_count: 5000,
      volume_usdc: "1234.56",
      by_band: {
        dust:  { txn_count: 1000, volume_usdc: "0.10" },
        micro: { txn_count: 1000, volume_usdc: "10.00" },
        small: { txn_count: 1000, volume_usdc: "100.00" },
        mid:   { txn_count: 1000, volume_usdc: "500.00" },
        whale: { txn_count: 1000, volume_usdc: "624.46" },
      },
    },
    "7d":  { txn_count: 100,  volume_usdc: "50.00",  by_band: { dust:{txn_count:20,volume_usdc:"0.02"}, micro:{txn_count:20,volume_usdc:"2.00"}, small:{txn_count:20,volume_usdc:"10.00"}, mid:{txn_count:20,volume_usdc:"15.00"}, whale:{txn_count:20,volume_usdc:"22.98"} } },
    "30d": { txn_count: 500,  volume_usdc: "200.00", by_band: { dust:{txn_count:100,volume_usdc:"0.10"}, micro:{txn_count:100,volume_usdc:"10.00"}, small:{txn_count:100,volume_usdc:"40.00"}, mid:{txn_count:100,volume_usdc:"49.90"}, whale:{txn_count:100,volume_usdc:"100.00"} } },
  },
  daily: [
    ["2026-06-12", 1000, 200.00, true],
    ["2026-06-13",  900, 150.00, true],
    ["2026-06-14",  800, 120.00, true],
    ["2026-06-15",  700, 100.00, true],
    ["2026-06-16",  600,  90.00, true],
    ["2026-06-17",  500,  80.00, true],
    ["2026-06-18",  400,  70.00, true],
  ],
  monthly: [
    { month: "2026-05", complete: true, txn_count: 3000, volume_usdc: "800.00" },
    { month: "2026-06", complete: true, txn_count: 5000, volume_usdc: "1234.56" },
  ],
  typical: {
    all:  { avg_usdc: "0.355771", median_usdc: "0.001", txn_count: 5000 },
    "7d": { avg_usdc: "0.50",     median_usdc: "0.25",  txn_count: 100 },
    "30d":{ avg_usdc: "0.40",     median_usdc: "0.10",  txn_count: 500 },
  },
  price_points: {
    all:  [{ amount_usdc: "0.001", txn_count: 3000, volume_usdc: "3.00", txn_share_pct: "60.00", payee_count: 50 }],
    "7d": [],
    "30d":[],
  },
  gas: {
    windows: {
      all:  { txn_count: 5000, breakeven_txn_count: 2500, gas_cents_per_dollar: "0.50",
               gas_eth: "0.01", gas_eth_l1: "0.005", gas_eth_l2: "0.005", gas_usd: "25.00", by_band: {} },
      "7d": { txn_count: 100,  breakeven_txn_count: 40,  gas_cents_per_dollar: "0.40",
               gas_eth: "0.001", gas_eth_l1: "0.0005", gas_eth_l2: "0.0005", gas_usd: "2.50", by_band: {} },
      "30d":{ txn_count: 500,  breakeven_txn_count: 200, gas_cents_per_dollar: "0.45",
               gas_eth: "0.005", gas_eth_l1: "0.0025", gas_eth_l2: "0.0025", gas_usd: "12.50", by_band: {} },
    },
    cost_daily: [],
  },
  velocity: {
    windows: { all: { max_per_min: 250 }, "7d": { max_per_min: 200 }, "30d": { max_per_min: 225 } },
    verified_daily: [
      ["2026-06-17", 250, 220],
      ["2026-06-18", 200, 180],
    ],
  },
  claims: [
    {
      claim_text: "Economy reached $10M",
      source: "TestSource",
      source_url: "https://example.com",
      claim_date: "2026-06-01",
      claimed_value: "10000000",
      measured_value: "1234567",
      measured_metric: "volume_usdc",
      measured_unit: "USDC",
    },
  ],
  concentration: {
    windows: {
      all:  { payee: { total_entities: 12345 }, payer: { total_entities: 9000 } },
      "7d": { payee: { total_entities: 500 },   payer: { total_entities: 400 } },
      "30d":{ payee: { total_entities: 3000 },  payer: { total_entities: 2000 } },
    },
  },
  excluded: { txn_count: 100000, volume_usdc: "456000000.00" },
  active_entities: [
    { day: "2026-06-17", payer_count: 1234, payee_count: 567, complete: true },
    { day: "2026-06-18", payer_count: 1100, payee_count: 500, complete: true },
  ],
  payer_cohorts: {
    "7d":  { new_payer_volume_usdc: "25.00",  returning_payer_volume_usdc: "25.00"  },
    "30d": { new_payer_volume_usdc: "80.00",  returning_payer_volume_usdc: "120.00" },
  },
  price_breadth: null,
};

const WIN_LABELS = { all: "all data Jan–Jun", "7d": "trailing 7d", "30d": "trailing 30d" };

function setup(win = "all") {
  setData(FIX);
  setWinLabel(WIN_LABELS);
  state.win = win;
  state.dWin = "all";
  state.dMetric = "tx";
  state.dScale = "lin";
  state.dMa = "raw";
  state.bMetric = "tx";
  _clearPins();
}

// ---------------------------------------------------------------------------
// OVERVIEW pinner ↔ rOverview panel
// ---------------------------------------------------------------------------
describe("8.3 — overview pinner ↔ panel equivalence", () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <span id="ov-win"></span>
      <div id="ov-stats"></div>
      <div id="ov-bands"></div>
      <div id="ov-denom"></div>`;
    setup("all");
  });

  it("pin.value is fmtUSDC(volume_usdc) and that exact string appears in panel", () => {
    rOverview();
    const pin = PINNERS.overview();
    const expectedVolume = fmtUSDC(FIX.windows.all.volume_usdc);
    expect(pin.value).toBe(expectedVolume);
    expect(document.getElementById("ov-stats").innerHTML).toContain(expectedVolume);
  });

  it("pin.context mentions txn_count and median: both appear in panel", () => {
    rOverview();
    const pin = PINNERS.overview();
    const expectedMedian = fmtUSDCAmt(FIX.typical.all.median_usdc);
    const expectedCount  = fmtCount(FIX.windows.all.txn_count);
    expect(pin.context).toContain(expectedMedian);
    expect(pin.context).toContain(expectedCount);
    expect(document.getElementById("ov-stats").innerHTML).toContain(expectedMedian);
    expect(document.getElementById("ov-stats").innerHTML).toContain(expectedCount);
  });

  it("pin.value changes with window (7d uses 7d volume)", () => {
    setup("7d");
    const pin7d = PINNERS.overview();
    setup("all");
    const pinAll = PINNERS.overview();
    expect(pin7d.value).not.toBe(pinAll.value);
    expect(pin7d.value).toContain(fmtUSDC(FIX.windows["7d"].volume_usdc));
  });
});

// ---------------------------------------------------------------------------
// SHAPE pinner ↔ rShape panel (catches percentile strip equivalence)
// ---------------------------------------------------------------------------
describe("8.3 — shape pinner ↔ panel equivalence", () => {
  function shapeDom() {
    document.body.innerHTML = `
      <span id="shp-win"></span>
      <div id="shp-big"></div>
      <div id="bands"></div>`;
  }

  beforeEach(() => { shapeDom(); setup("all"); });

  it("pin.value starts with fmtUSDCAmt(median) which also appears in panel #shp-big", () => {
    rShape();
    const pin = PINNERS.shape();
    const expectedMedian = fmtUSDCAmt(FIX.typical.all.median_usdc);
    expect(pin.value).toContain(expectedMedian);
    expect(document.getElementById("shp-big").innerHTML).toContain(expectedMedian);
  });

  it("fmtUSDCAmt(avg) appears identically in both pin.context and panel #shp-big", () => {
    rShape();
    const pin = PINNERS.shape();
    const expectedAvg = fmtUSDCAmt(FIX.typical.all.avg_usdc);
    expect(pin.context).toContain(expectedAvg);
    expect(document.getElementById("shp-big").innerHTML).toContain(expectedAvg);
  });

  it("pin.value is exactly fmtUSDCAmt(median)+' median'", () => {
    const pin = PINNERS.shape();
    expect(pin.value).toBe(fmtUSDCAmt(FIX.typical.all.median_usdc) + " median");
  });

  it("percentile values appear identically in pin.context and panel when p10/p90/p99 present", () => {
    // Set up fixture with explicit percentile fields
    const fixWithPct = {
      ...FIX,
      typical: {
        ...FIX.typical,
        all: { ...FIX.typical.all, p10_usdc: "0.0001", p90_usdc: "0.50", p99_usdc: "1.00" },
      },
    };
    setData(fixWithPct);
    shapeDom();
    rShape();
    const pin = PINNERS.shape();
    const p10 = fmtUSDCAmt("0.0001");
    const p90 = fmtUSDCAmt("0.50");
    const p99 = fmtUSDCAmt("1.00");
    // All three formatted values appear in pin.context
    expect(pin.context).toContain(p10);
    expect(pin.context).toContain(p90);
    expect(pin.context).toContain(p99);
    // All three formatted values appear in the panel strip
    const panelHtml = document.getElementById("shp-big").innerHTML;
    expect(panelHtml).toContain(p10);
    expect(panelHtml).toContain(p90);
    expect(panelHtml).toContain(p99);
  });

  it("pin returns null when txn_count is 0 (empty window)", () => {
    const fixEmpty = {
      ...FIX,
      typical: { ...FIX.typical, all: { avg_usdc: "0", median_usdc: "0", txn_count: 0 } },
    };
    setData(fixEmpty);
    const pin = PINNERS.shape();
    expect(pin).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// GAS pinner ↔ rGas panel (percentage must match exactly)
// ---------------------------------------------------------------------------
describe("8.3 — gas pinner ↔ panel equivalence", () => {
  function gasDom() {
    document.body.innerHTML = `
      <span id="gas-win"></span>
      <div id="gas-pct"></div>
      <div id="gas-meter"></div>
      <div id="gas-kv"></div>
      <table id="gasbands"></table>`;
  }

  beforeEach(() => { gasDom(); setup("all"); });

  it("pin.value starts with the same percentage that panel writes to #gas-pct", () => {
    rGas();
    const pin = PINNERS.gas();
    const g = FIX.gas.windows.all;
    const expectedPct = (100 * g.breakeven_txn_count / g.txn_count).toFixed(1) + "%";
    // Panel sets textContent of #gas-pct to just the percentage
    const panelPct = document.getElementById("gas-pct").textContent;
    expect(panelPct).toBe(expectedPct);
    // Pinner adds " cost>value" suffix but must contain the same percentage
    expect(pin.value).toContain(expectedPct);
  });

  it("gas pinner returns null when txn_count is 0", () => {
    const fixNoGas = {
      ...FIX,
      gas: {
        ...FIX.gas,
        windows: {
          ...FIX.gas.windows,
          all: { txn_count: 0, breakeven_txn_count: 0, gas_cents_per_dollar: null,
                 gas_eth: "0", gas_eth_l1: "0", gas_eth_l2: "0", gas_usd: "0", by_band: {} },
        },
      },
    };
    setData(fixNoGas);
    const pin = PINNERS.gas();
    expect(pin).toBeNull();
  });

  it("breakeven count appears identically in pin.context and panel #gas-kv", () => {
    rGas();
    const pin = PINNERS.gas();
    const g = FIX.gas.windows.all;
    const expectedBreakeven = fmtInt(g.breakeven_txn_count);
    expect(pin.context).toContain(expectedBreakeven);
    expect(document.getElementById("gas-kv").innerHTML).toContain(expectedBreakeven);
  });
});

// ---------------------------------------------------------------------------
// PRICE pinner ↔ rPrice panel (share percentage must match panel rendering)
// ---------------------------------------------------------------------------
describe("8.3 — price pinner ↔ panel share equivalence", () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <span id="pp-win"></span>
      <table id="pptable"></table>`;
    setup("all");
    // Import and call rPrice - done inline to avoid async import
  });

  it("pin.context share percentage uses .toFixed(1) matching the panel's share column", async () => {
    const { rPrice } = await import("../app/assets/js/panels.js");
    rPrice();
    const pin = PINNERS.price();
    const p = FIX.price_points.all[0];
    const expectedShare = parseFloat(p.txn_share_pct).toFixed(1) + "%";
    // Both pin and panel must contain the same formatted share
    expect(pin.context).toContain(expectedShare);
    expect(document.getElementById("pptable").innerHTML).toContain(expectedShare);
  });

  it("pin.value contains fmtUSDCAmt(amount_usdc) that also appears in panel table", async () => {
    const { rPrice } = await import("../app/assets/js/panels.js");
    rPrice();
    const pin = PINNERS.price();
    const p = FIX.price_points.all[0];
    const expectedAmt = fmtUSDCAmt(p.amount_usdc);
    expect(pin.value).toContain(expectedAmt);
    expect(document.getElementById("pptable").innerHTML).toContain(expectedAmt);
  });

  it("pin.value contains same txn_count format as panel tx column", async () => {
    const { rPrice } = await import("../app/assets/js/panels.js");
    rPrice();
    const pin = PINNERS.price();
    const p = FIX.price_points.all[0];
    // Panel and pinner must both format txn_count with fmtInt — character-identical.
    const panelCount = fmtInt(p.txn_count);
    expect(document.getElementById("pptable").innerHTML).toContain(panelCount);
    expect(pin.value).toContain(panelCount);
  });
});

// ---------------------------------------------------------------------------
// CLAIMS pinner ↔ rClaims panel (verdict string must be character-identical)
// ---------------------------------------------------------------------------
describe("8.3 — claims pinner ↔ panel verdict equivalence", () => {
  beforeEach(() => {
    document.body.innerHTML = `<div id="claims"></div>`;
    setup("all");
  });

  it("pin.value contains same claimVerdict as panel tag text", () => {
    rClaims();
    const pin = PINNERS.claims();
    const c = FIX.claims[0];
    const ratio = parseFloat(c.claimed_value) / parseFloat(c.measured_value);
    const expectedVerdict = claimVerdict(ratio);
    // Panel: "claim <verdict>" inside a tag; pinner: "claim <verdict>"
    expect(pin.value).toContain("claim " + expectedVerdict);
    expect(document.getElementById("claims").innerHTML).toContain("claim " + expectedVerdict);
  });

  it("pin.context escapes HTML in claim_text (same escaping as panel)", () => {
    const fixHtml = {
      ...FIX,
      claims: [{
        ...FIX.claims[0],
        claim_text: "<b>Test</b> claim",
      }],
    };
    setData(fixHtml);
    rClaims();
    const pin = PINNERS.claims();
    // Both should have escaped HTML
    expect(pin.context).toContain("&lt;b&gt;");
    expect(document.getElementById("claims").innerHTML).toContain("&lt;b&gt;");
  });

  it("pin returns null when claims list is empty", () => {
    const fixNoClaims = { ...FIX, claims: [] };
    setData(fixNoClaims);
    const pin = PINNERS.claims();
    expect(pin).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// DAILY pinner — peak follows the active metric (regression guard)
// ---------------------------------------------------------------------------
describe("8.3 — daily pinner peak follows active metric", () => {
  // tx-peak: day 0 (tx=1000), usd-peak: day 0 (usd=200 is also day 0 here)
  // Use fixture where tx and usd peaks are on different days:
  const dailyFixture = {
    ...FIX,
    daily: [
      ["2026-06-12", 1000,  10.00, true],  // tx peak
      ["2026-06-13",  900,  20.00, true],
      ["2026-06-14",  800, 200.00, true],  // usd peak
    ],
  };

  beforeEach(() => {
    setData(dailyFixture);
    setWinLabel(WIN_LABELS);
    state.dWin = "all";
    _clearPins();
  });

  it("TX mode: pin.value contains fmtInt(tx-peak) and pin.context shows tx-peak day", () => {
    state.dMetric = "tx";
    const pin = PINNERS.daily();
    expect(pin.value).toContain("1,000");        // tx peak count
    expect(pin.context).toContain("2026-06-12"); // tx peak day
  });

  it("USD mode: pin.value is money-formatted and pin.context shows usd-peak day", () => {
    state.dMetric = "usd";
    const pin = PINNERS.daily();
    expect(pin.value).toContain("USDC");            // money format
    expect(pin.value).not.toContain("tx/day");     // not the tx label
    expect(pin.context).toContain("2026-06-14");   // usd peak day
  });

  it("USD mode: pin.context does NOT show the tx-peak day", () => {
    state.dMetric = "usd";
    const pin = PINNERS.daily();
    expect(pin.context).not.toContain("2026-06-12");
  });
});

// ---------------------------------------------------------------------------
// ACTIVE_WALLETS pinner — value/context reflect last complete day
// ---------------------------------------------------------------------------
describe("8.3 — active_wallets pinner ↔ data", () => {
  beforeEach(() => { setup("all"); });

  it("pin.value contains fmtInt(payer_count) and fmtInt(payee_count) from last complete day", () => {
    const pin = PINNERS.active_wallets();
    const lastComplete = FIX.active_entities.filter(p => p.complete).at(-1);
    expect(pin.value).toContain(fmtInt(lastComplete.payer_count));
    expect(pin.value).toContain(fmtInt(lastComplete.payee_count));
  });

  it("pin returns null when active_entities is absent", () => {
    const fixNoAe = { ...FIX, active_entities: null };
    setData(fixNoAe);
    const pin = PINNERS.active_wallets();
    expect(pin).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// VELOCITY pinner ↔ data
// ---------------------------------------------------------------------------
describe("8.3 — velocity pinner ↔ data", () => {
  beforeEach(() => { setup("all"); });

  it("pin.value contains fmtInt(max_per_min)+'/min' from windows.all", () => {
    const pin = PINNERS.velocity();
    expect(pin.value).toContain(fmtInt(FIX.velocity.windows.all.max_per_min) + "/min");
  });

  it("pin returns null when verified_daily is empty", () => {
    const fixNoVel = {
      ...FIX,
      velocity: { ...FIX.velocity, verified_daily: [] },
    };
    setData(fixNoVel);
    const pin = PINNERS.velocity();
    expect(pin).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// FACILITATORS pinner — requires facData.rows with windows
// ---------------------------------------------------------------------------
describe("8.3 — facilitators pinner", () => {
  beforeEach(() => {
    setup("all");
  });

  it("pin returns null when facData is null (not loaded)", () => {
    setFacData(null);
    const pin = PINNERS.facilitators();
    expect(pin).toBeNull();
  });

  it("pin returns null when rows lack window data (current artifact state)", () => {
    setFacData({ rows: [{ facilitator: "0xabc", txn_count: 100, volume_usdc: "500.00" }] });
    const pin = PINNERS.facilitators();
    expect(pin).toBeNull();
  });

  it("pin returns non-null when rows have 7d/30d windows", () => {
    setFacData({
      rows: [{
        facilitator: "0xabc1234567890",
        txn_count: 100,
        volume_usdc: "500.00",
        windows: {
          "7d":  { volume_usdc: "50.00" },
          "30d": { volume_usdc: "200.00" },
        },
      }],
    });
    const pin = PINNERS.facilitators();
    expect(pin).not.toBeNull();
    expect(pin.value).toContain(fmtUSDC("500.00"));
  });
});

// ---------------------------------------------------------------------------
// MONTHLY pinner — value/context use MoM of last two complete months
// ---------------------------------------------------------------------------
describe("8.3 — monthly pinner ↔ data", () => {
  beforeEach(() => { setup("all"); });

  it("pin returns non-null and value describes MoM change", () => {
    const pin = PINNERS.monthly();
    expect(pin).not.toBeNull();
    expect(pin.value).toMatch(/[A-Z][a-z]{2} USDC: [+-]\d+% MoM/);
  });

  it("pin returns 'insufficient complete months' value when < 2 complete months", () => {
    const fixOneMonth = {
      ...FIX,
      monthly: [{ month: "2026-06", complete: true, txn_count: 5000, volume_usdc: "1234.56" }],
    };
    setData(fixOneMonth);
    const pin = PINNERS.monthly();
    expect(pin.value).toContain("insufficient complete months");
  });
});
