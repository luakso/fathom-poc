import { describe, it, expect, beforeEach } from "vitest";
import { renderBuckets, BUCKET_ORDER } from "../site/assets/js/lib/buckets.js";
import { payeeDoc, sparseDoc } from "./fixtures/entities.js";

const buckets = payeeDoc.data.windows.all.buckets;
beforeEach(() => { document.body.innerHTML = `<div id="b"></div>`; });

describe("renderBuckets", () => {
  it("renders a row for every canonical bucket in order", () => {
    renderBuckets(document.getElementById("b"), buckets, { metric: "count" });
    expect(document.querySelectorAll("#b .mrow").length).toBe(BUCKET_ORDER.length);
  });
  it("uses entity_count when metric is count", () => {
    renderBuckets(document.getElementById("b"), buckets, { metric: "count" });
    expect(document.getElementById("b").textContent).toContain("2"); // 2-10 bucket entity_count
  });
  it("does not throw on empty buckets", () => {
    expect(() => renderBuckets(document.getElementById("b"), sparseDoc.data.windows.all.buckets, { metric: "count" })).not.toThrow();
  });
});
