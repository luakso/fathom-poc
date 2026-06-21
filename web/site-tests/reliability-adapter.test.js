import { describe, it, expect } from "vitest";
import {
  reshapeReliability, checkReliabilityIntegrity, crossCheckEconomyReliability, latencyBars, LAT_BUCKETS,
} from "../site/assets/js/lib/reliability-adapter.js";
import { reliabilityDoc, economyDoc } from "./fixtures/reliability.js";

describe("reshapeReliability", () => {
  it("lifts meta and keeps windows/daily/attribution", () => {
    const v = reshapeReliability(reliabilityDoc);
    expect(v.meta.data_through_day).toBe("2026-06-06");
    expect(Object.keys(v.windows)).toContain("all");
    expect(v.daily).toHaveLength(2);
    expect(v.attribution.by_payer).toHaveLength(1);
  });
});

describe("checkReliabilityIntegrity", () => {
  it("passes clean on a reconciling fixture", () => {
    const issues = checkReliabilityIntegrity(reshapeReliability(reliabilityDoc));
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("flags windowed_count > settlement_count", () => {
    const broken = structuredClone(reliabilityDoc);
    broken.data.windows.all.windowed_count = 999;
    const issues = checkReliabilityIntegrity(reshapeReliability(broken));
    expect(issues.some(i => i.level === "error" && /windowed/.test(i.msg))).toBe(true);
  });
  it("flags membership not reconciling to settlement_count", () => {
    const broken = structuredClone(reliabilityDoc);
    broken.data.windows.all.by_membership.known.settlement_count = 5;
    const issues = checkReliabilityIntegrity(reshapeReliability(broken));
    expect(issues.some(i => i.level === "error" && /known\+unknown/.test(i.msg))).toBe(true);
  });
  it("flags a rate outside [0,1]", () => {
    const broken = structuredClone(reliabilityDoc);
    broken.data.windows.all.expired_rate = 1.5;
    const issues = checkReliabilityIntegrity(reshapeReliability(broken));
    expect(issues.some(i => i.level === "error" && /expired_rate/.test(i.msg))).toBe(true);
  });
  it("warns on an unexpected methodology version", () => {
    const broken = structuredClone(reliabilityDoc);
    broken.methodology_version = 2;
    const issues = checkReliabilityIntegrity(reshapeReliability(broken));
    expect(issues.some(i => i.level === "warn" && /methodology/.test(i.msg))).toBe(true);
  });
});

describe("crossCheckEconomyReliability", () => {
  it("passes when settlement_count == economy txn_count", () => {
    const issues = crossCheckEconomyReliability(reshapeReliability(reliabilityDoc), economyDoc);
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("flags a mismatch", () => {
    const bad = structuredClone(economyDoc);
    bad.data.windows.all.txn_count = 99;
    const issues = crossCheckEconomyReliability(reshapeReliability(reliabilityDoc), bad);
    expect(issues.some(i => i.level === "error" && /cross-check/.test(i.msg))).toBe(true);
  });
  it("warns (not errors) when economy is missing", () => {
    const issues = crossCheckEconomyReliability(reshapeReliability(reliabilityDoc), { data: {} });
    expect(issues.every(i => i.level === "warn")).toBe(true);
  });
});

describe("latencyBars", () => {
  it("returns 5 ordered bars with counts and fractions summing to 1", () => {
    const bars = latencyBars(reliabilityDoc.data.windows.all.latency.buckets);
    expect(bars.map(b => b.label)).toEqual(LAT_BUCKETS.map(b => b.label));
    expect(bars.reduce((s, b) => s + b.count, 0)).toBe(80);
    expect(bars.reduce((s, b) => s + b.frac, 0)).toBeCloseTo(1, 6);
  });
  it("returns all-zero fractions on an empty histogram (no divide-by-zero)", () => {
    const bars = latencyBars({ sub1s: 0, "1_10s": 0, "10_60s": 0, "1_10m": 0, gt10m: 0 });
    expect(bars.every(b => b.frac === 0)).toBe(true);
  });
});
