import { describe, it, expect } from "vitest";
import { tapeSlice } from "../site/assets/js/charts.js";

// 120 days of compact tape rows: [day, txn_count, volume_cents]
const days = Array.from({ length: 120 }, (_, i) => {
  const d = new Date(Date.UTC(2026, 0, 1 + i)).toISOString().slice(0, 10);
  return [d, i, i * 100];
});

describe("tapeSlice", () => {
  it("returns the trailing 7 days for 7d", () => {
    const s = tapeSlice(days, "7d");
    expect(s.length).toBe(7);
    expect(s[s.length - 1]).toBe(days[days.length - 1]);
    expect(s[0]).toBe(days[days.length - 7]);
  });
  it("returns the trailing 30 days for 30d", () => {
    expect(tapeSlice(days, "30d").length).toBe(30);
  });
  it("returns the trailing 90 days for 90d", () => {
    const s = tapeSlice(days, "90d");
    expect(s.length).toBe(90);
    expect(s[0]).toBe(days[30]);
  });
  it("returns the full series for all", () => {
    expect(tapeSlice(days, "all")).toBe(days);
  });
  it("never over-slices when the window exceeds the data length", () => {
    const short = days.slice(0, 10);
    expect(tapeSlice(short, "30d")).toEqual(short);
    expect(tapeSlice(short, "90d")).toEqual(short);
  });
});
