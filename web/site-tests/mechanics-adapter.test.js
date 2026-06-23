import { describe, it, expect } from "vitest";
import {
  reshapeMechanics, checkMechanicsIntegrity, crossCheckEconomyMechanics,
  selectorLabel, txTypeOther,
} from "../site/assets/js/lib/mechanics-adapter.js";
import { mechanicsDoc, economyDoc } from "./fixtures/mechanics.js";

describe("reshapeMechanics", () => {
  it("lifts meta + windows", () => {
    const v = reshapeMechanics(mechanicsDoc);
    expect(v.meta.data_through_day).toBe("2026-06-06");
    expect(Object.keys(v.windows)).toContain("all");
  });
});

describe("checkMechanicsIntegrity", () => {
  it("passes clean on a reconciling fixture (tx_type 'other' + top-15 selector are NOT errors)", () => {
    const issues = checkMechanicsIntegrity(reshapeMechanics(mechanicsDoc));
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("flags membership not reconciling", () => {
    const b = structuredClone(mechanicsDoc);
    b.data.windows.all.by_membership.known.settlement_count = 1;
    const issues = checkMechanicsIntegrity(reshapeMechanics(b));
    expect(issues.some(i => i.level === "error" && /known\+unknown/.test(i.msg))).toBe(true);
  });
  it("flags batch payment_count not summing to settlements", () => {
    const b = structuredClone(mechanicsDoc);
    b.data.windows.all.batch.histogram[0].payment_count = 999;
    const issues = checkMechanicsIntegrity(reshapeMechanics(b));
    expect(issues.some(i => i.level === "error" && /batch/.test(i.msg))).toBe(true);
  });
  it("flags pct_batched outside [0,1]", () => {
    const b = structuredClone(mechanicsDoc);
    b.data.windows.all.batch.pct_batched = 1.5;
    const issues = checkMechanicsIntegrity(reshapeMechanics(b));
    expect(issues.some(i => i.level === "error" && /pct_batched/.test(i.msg))).toBe(true);
  });
  it("does NOT flag the tx_type 'other' remainder", () => {
    // fixture tx_type sums to 9 < settlement 10 → 1 other; must be clean
    const issues = checkMechanicsIntegrity(reshapeMechanics(mechanicsDoc));
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
});

describe("crossCheckEconomyMechanics", () => {
  it("passes when settlement_count == economy txn_count", () => {
    const issues = crossCheckEconomyMechanics(reshapeMechanics(mechanicsDoc), economyDoc);
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("flags a mismatch", () => {
    const bad = structuredClone(economyDoc); bad.data.windows.all.txn_count = 99;
    const issues = crossCheckEconomyMechanics(reshapeMechanics(mechanicsDoc), bad);
    expect(issues.some(i => i.level === "error" && /cross-check/.test(i.msg))).toBe(true);
  });
  it("warns when economy missing", () => {
    const issues = crossCheckEconomyMechanics(reshapeMechanics(mechanicsDoc), { data: {} });
    expect(issues.every(i => i.level === "warn")).toBe(true);
  });
});

describe("selectorLabel + txTypeOther", () => {
  it("labels known selectors, null for unknown", () => {
    expect(selectorLabel("e3ee160e")).toMatch(/transferWithAuthorization/);
    expect(selectorLabel("82ad56cb")).toMatch(/Multicall3/);
    expect(selectorLabel("deadbeef")).toBeNull();
  });
  it("computes the tx_type other remainder", () => {
    const m = reshapeMechanics(mechanicsDoc).windows.all;
    expect(txTypeOther(m)).toBe(1); // 10 - (1+0+8)
  });
});
