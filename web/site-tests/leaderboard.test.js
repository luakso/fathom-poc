import { describe, it, expect, beforeEach } from "vitest";
import { renderLeaderboard, shortAddr } from "../site/assets/js/lib/leaderboard.js";
import { payeeDoc, sparseDoc } from "./fixtures/entities.js";

const rows = payeeDoc.data.windows.all.leaderboard;

beforeEach(() => { document.body.innerHTML = `<table id="t"></table>`; });

describe("shortAddr", () => {
  it("truncates the middle", () => {
    expect(shortAddr("0x1234567890abcdef")).toBe("0x1234…cdef");
  });
});

describe("renderLeaderboard", () => {
  it("renders one row per leaderboard entry with a fingerprint badge", () => {
    renderLeaderboard(document.getElementById("t"), rows, { cpartyLabel: "payers" });
    const trs = document.querySelectorAll("#t tbody tr");
    expect(trs).toHaveLength(rows.length);
    expect(document.querySelectorAll("#t .fp").length).toBe(rows.length);
  });
  it("sorts by volume descending by default (largest first)", () => {
    renderLeaderboard(document.getElementById("t"), rows, { cpartyLabel: "payers" });
    const first = document.querySelector("#t tbody tr td:nth-child(2)").textContent;
    expect(first).toContain("$10.00M"); // the otc row, $10,000,000
  });
  it("re-sorts ascending when a header is clicked twice", () => {
    renderLeaderboard(document.getElementById("t"), rows, { cpartyLabel: "payers" });
    const th = [...document.querySelectorAll("#t thead th")].find(h => /txns/i.test(h.textContent));
    th.click(); th.click(); // asc
    const firstTxns = document.querySelector("#t tbody tr td:nth-child(3)").textContent.trim();
    expect(firstTxns).toBe("2"); // the otc row has the fewest txns
  });
  it("shows an empty-state row when there are no entries", () => {
    renderLeaderboard(document.getElementById("t"), [], { cpartyLabel: "payers" });
    expect(document.querySelector("#t tbody").textContent).toMatch(/no .* in this window/i);
  });
  it("invokes onPin with the row when a pin control is clicked", () => {
    let pinned = null;
    renderLeaderboard(document.getElementById("t"), rows, { cpartyLabel: "payers", onPin: r => (pinned = r) });
    document.querySelector("#t [data-pin-row]").click();
    expect(pinned).toBeTruthy();
    expect(pinned.address).toBeDefined();
  });
});

it("does not throw on the sparse fixture", () => {
  document.body.innerHTML = `<table id="t"></table>`;
  expect(() => renderLeaderboard(document.getElementById("t"), sparseDoc.data.windows.all.leaderboard, { cpartyLabel: "payers" })).not.toThrow();
});
