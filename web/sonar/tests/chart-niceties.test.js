// Tests for item 7.3 - Chart niceties.
// Covers: log-mode fallback gridlines when fewer than 2 decade lines visible;
// velocity peak annotation flips anchor near right edge.
import { describe, it, expect, beforeEach } from "vitest";
import { state, setData, setWinLabel } from "../app/assets/js/state.js";

// ---------------------------------------------------------------------------
// Shared DOM helpers
// ---------------------------------------------------------------------------
function chartDom() {
  document.body.innerHTML = `
    <div id="dailychart" style="width:900px"></div>
    <div id="velochart" style="width:800px"></div>
    <div id="velostats"></div>
    <div id="v-readout"></div>
    <span id="d-range"></span>
    <span id="d-partial-note"></span>`;
}

// Daily series whose range stays within a single decade (5k-48.5k).
// This sits between 10^3 and 10^5 so only one decade line (10^4) falls inside.
function makeSingleDecadeSeries(count = 30) {
  return Array.from({ length: count }, (_, i) => [
    `2026-06-${String(i + 1).padStart(2, "0")}`,
    5000 + i * 1500,
    (5000 + i * 1500) * 0.1,
  ]);
}

// Velocity series with the peak at the LAST day.
function makePeakAtEndVelocity(count = 30) {
  return Array.from({ length: count }, (_, i) => {
    const isLast = i === count - 1;
    const max = isLast ? 1000 : 50 + i;
    return [`2026-06-${String(i + 1).padStart(2, "0")}`, max, Math.round(max * 0.8)];
  });
}

// Velocity series with the peak at the FIRST day.
function makePeakAtStartVelocity(count = 30) {
  return Array.from({ length: count }, (_, i) => {
    const isFirst = i === 0;
    const max = isFirst ? 1000 : 50 + i;
    return [`2026-06-${String(i + 1).padStart(2, "0")}`, max, Math.round(max * 0.8)];
  });
}

const baseData = (daily, velDaily) => ({
  meta: { data_through_day: "2026-06-30", methodology_version: 1 },
  windows: {
    all:  { txn_count: 1000, volume_usdc: "1000", by_band: {} },
    "7d": { txn_count: 100,  volume_usdc: "100",  by_band: {} },
    "30d":{ txn_count: 500,  volume_usdc: "500",  by_band: {} },
  },
  daily: daily || makeSingleDecadeSeries(),
  monthly: [],
  typical: {
    all:  { avg_usdc: "1", median_usdc: "0.5", txn_count: 1000 },
    "7d": { avg_usdc: "1", median_usdc: "0.5", txn_count: 100 },
    "30d":{ avg_usdc: "1", median_usdc: "0.5", txn_count: 500 },
  },
  price_points: { all: [], "7d": [], "30d": [] },
  gas: { windows: {
    all:  { txn_count: 1000, breakeven_txn_count: 500, gas_cents_per_dollar: "0.50" },
    "7d": { txn_count: 100,  breakeven_txn_count: 50,  gas_cents_per_dollar: "0.50" },
    "30d":{ txn_count: 500,  breakeven_txn_count: 250, gas_cents_per_dollar: "0.50" },
  }},
  velocity: {
    windows: { all: { max_per_min: 1000 }, "7d": { max_per_min: 800 }, "30d": { max_per_min: 900 } },
    verified_daily: velDaily || [],
  },
  claims: [],
  concentration: { windows: {
    all:  { payee: { total_entities: 100 } },
    "7d": { payee: { total_entities: 20 } },
    "30d":{ payee: { total_entities: 50 } },
  }},
});

// ---------------------------------------------------------------------------
// 7.3a - Log-mode fallback gridlines
// ---------------------------------------------------------------------------
describe("7.3a - log fallback gridlines when data spans less than 2 decades", () => {
  beforeEach(() => {
    chartDom();
    setData(baseData(makeSingleDecadeSeries(30)));
    setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" });
    state.dWin = "all";
    state.dMetric = "tx";
    state.dMa = "raw";
  });

  it("log mode with sub-decade range produces at least 2 y-axis gridline labels", async () => {
    state.dScale = "log";
    const { rDaily } = await import("../app/assets/js/charts.js");
    rDaily();
    const svg = document.querySelector("#dailychart svg");
    expect(svg).not.toBeNull();
    // Count text elements with text-anchor="end" AND positioned on the left
    // (y-axis labels). padL=56, so their x attr is "49" (padL-7).
    const texts = Array.from(svg.querySelectorAll("text[text-anchor='end']"));
    // Some of these may be x-axis trailing ticks; y-axis ones have x near 49.
    const yLabels = texts.filter(t => {
      const xAttr = parseFloat(t.getAttribute("x") || "0");
      return xAttr < 60; // padL-7 = 49
    });
    expect(yLabels.length).toBeGreaterThanOrEqual(2);
  });

  it("lin mode retains 3 y-axis gridline labels (no regression)", async () => {
    state.dScale = "lin";
    const { rDaily } = await import("../app/assets/js/charts.js");
    rDaily();
    const svg = document.querySelector("#dailychart svg");
    const texts = Array.from(svg.querySelectorAll("text[text-anchor='end']"));
    const yLabels = texts.filter(t => parseFloat(t.getAttribute("x") || "0") < 60);
    expect(yLabels.length).toBe(3);
  });
});

// ---------------------------------------------------------------------------
// 7.3b - Velocity peak annotation flips anchor near right edge
// ---------------------------------------------------------------------------
describe("7.3b - velocity peak annotation clips near right edge", () => {
  beforeEach(() => { chartDom(); setWinLabel({ all: "all data", "7d": "trailing 7d", "30d": "trailing 30d" }); });

  it("peak annotation has text-anchor='end' when peak is at the last day", async () => {
    setData(baseData(null, makePeakAtEndVelocity(30)));
    const { rVelocity } = await import("../app/assets/js/charts.js");
    rVelocity();
    const svg = document.querySelector("#velochart svg");
    expect(svg).not.toBeNull();
    // The peak annotation: styled with --contam and shows /min
    const texts = Array.from(svg.querySelectorAll("text"));
    const ann = texts.find(t => t.textContent.includes("/min") && (t.getAttribute("style") || "").includes("contam"));
    expect(ann).not.toBeNull();
    expect(ann.getAttribute("text-anchor")).toBe("end");
  });

  it("peak annotation has no text-anchor (left-anchored) when peak is at the first day", async () => {
    setData(baseData(null, makePeakAtStartVelocity(30)));
    const { rVelocity } = await import("../app/assets/js/charts.js");
    rVelocity();
    const svg = document.querySelector("#velochart svg");
    const texts = Array.from(svg.querySelectorAll("text"));
    const ann = texts.find(t => t.textContent.includes("/min") && (t.getAttribute("style") || "").includes("contam"));
    expect(ann).not.toBeNull();
    // When the peak is near the left edge, no text-anchor or text-anchor != "end"
    expect(ann.getAttribute("text-anchor")).not.toBe("end");
  });
});
