import { describe, it, expect, beforeEach } from "vitest";
import { renderScatter, classCounts } from "../site/assets/js/lib/scatter.js";
import { payeeDoc, sparseDoc } from "./fixtures/entities.js";

const rows = payeeDoc.data.windows.all.leaderboard;

beforeEach(() => { document.body.innerHTML = `<div id="h" style="width:700px"></div>`; });

describe("renderScatter", () => {
  it("draws one circle per row", () => {
    renderScatter(document.getElementById("h"), rows, { xLabel: "payers" });
    expect(document.querySelectorAll("#h circle").length).toBe(rows.length);
  });
  it("labels the x axis with the role-specific counterparty name", () => {
    renderScatter(document.getElementById("h"), rows, { xLabel: "payers" });
    expect(document.getElementById("h").textContent).toContain("payers");
  });
  it("renders an empty-state note on the sparse fixture", () => {
    renderScatter(document.getElementById("h"), sparseDoc.data.windows.all.leaderboard, { xLabel: "payers" });
    expect(document.querySelectorAll("#h circle").length).toBe(0);
    expect(document.getElementById("h").textContent).toMatch(/no entities/i);
  });
});

describe("classCounts", () => {
  it("tallies count and volume per class", () => {
    const t = classCounts(rows);
    expect(t.service.count).toBe(1);
    expect(t.otc.volume).toBeCloseTo(10000000, 0);
  });
});
