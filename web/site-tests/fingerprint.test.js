import { describe, it, expect } from "vitest";
import { classify, CLASSES, FP } from "../site/assets/js/lib/fingerprint.js";
import { payeeDoc } from "./fixtures/entities.js";

// Note: spec listed slice(0,8) but fixture address prefixes are 9-char identifiers
// for all non-service classes; using slice(0,9) makes all five keys resolve correctly.
const rows = Object.fromEntries(
  payeeDoc.data.windows.all.leaderboard.map(r => [r.address.slice(0, 9), r])
);

describe("classify", () => {
  it("labels a broad many-customer payee as service", () => {
    expect(classify(rows["0xservice"])).toBe("service");
  });
  it("labels few-txn huge-average as otc", () => {
    expect(classify(rows["0xotc0000"])).toBe("otc");
  });
  it("labels high-volume tiny-average as sink", () => {
    expect(classify(rows["0xsink000"])).toBe("sink");
  });
  it("labels high-txn single-amount non-dust as fleet", () => {
    expect(classify(rows["0xfleet00"])).toBe("fleet");
  });
  it("labels small ambiguous rows as tail", () => {
    expect(classify(rows["0xtail000"])).toBe("tail");
  });
  it("exposes a label, color and blurb for every class key", () => {
    for (const key of ["otc", "sink", "fleet", "service", "tail"]) {
      expect(CLASSES[key]).toMatchObject({
        label: expect.any(String), color: expect.any(String), blurb: expect.any(String),
      });
    }
  });
  it("keeps thresholds in one tunable place", () => {
    expect(FP.SERVICE_MIN_CPARTIES).toBeGreaterThan(0);
  });
  it("never divides by zero on a zero-txn row", () => {
    expect(classify({ txn_count: 0, distinct_counterparties: 0, distinct_amounts: 0, volume_usdc: "0" })).toBe("tail");
  });
});
