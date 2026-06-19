import { describe, it, expect } from "vitest";
import { reshapeEntity, checkEntityIntegrity, crossCheckEconomy, USD_TOLERANCE }
  from "../site/assets/js/lib/entity-adapter.js";
import { payeeDoc, sparseDoc, economyDoc } from "./fixtures/entities.js";

describe("reshapeEntity", () => {
  it("lifts meta and keeps windows + role", () => {
    const v = reshapeEntity(payeeDoc);
    expect(v.role).toBe("payee");
    expect(v.meta.data_through_day).toBe("2026-06-06");
    expect(Object.keys(v.windows)).toContain("all");
  });
});

describe("checkEntityIntegrity", () => {
  it("returns no errors when buckets reconcile to concentration", () => {
    const issues = checkEntityIntegrity(reshapeEntity(payeeDoc));
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("flags an error when bucket txns drift from total", () => {
    const broken = structuredClone(payeeDoc);
    broken.data.windows.all.buckets[0].txn_sum += 5;
    const issues = checkEntityIntegrity(reshapeEntity(broken));
    expect(issues.some(i => i.level === "error" && /bucket txns/.test(i.msg))).toBe(true);
  });
  it("passes cleanly on a sparse (empty) window", () => {
    const issues = checkEntityIntegrity(reshapeEntity(sparseDoc));
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("warns on an unexpected methodology version", () => {
    const v2 = structuredClone(payeeDoc); v2.methodology_version = 2;
    const issues = checkEntityIntegrity(reshapeEntity(v2));
    expect(issues.some(i => i.level === "warn")).toBe(true);
  });
});

describe("crossCheckEconomy", () => {
  it("passes when page concentration equals economy's block for the role", () => {
    const issues = crossCheckEconomy(reshapeEntity(payeeDoc), economyDoc);
    expect(issues.filter(i => i.level === "error")).toHaveLength(0);
  });
  it("errors when economy disagrees on totals", () => {
    const econ = structuredClone(economyDoc);
    econ.data.concentration.windows.all.payee.total_txns += 1;
    const issues = crossCheckEconomy(reshapeEntity(payeeDoc), econ);
    expect(issues.some(i => i.level === "error")).toBe(true);
  });
});

it("exposes a shared USD tolerance", () => {
  expect(USD_TOLERANCE).toBeGreaterThan(0);
});
