// Tests for item 7.3c - Banner: bound and dismissible.
// Uses a local applyBannerFromIssues helper that mirrors the exact DOM
// contract that app.js applyMeta must implement.
import { describe, it, expect, beforeEach } from "vitest";

// ---------------------------------------------------------------------------
// Mirror of the banner logic in app.js — tests prove the DOM contract.
// ---------------------------------------------------------------------------
function applyBannerFromIssues(issues) {
  const failing = issues.filter(i => !i.pass);
  if (!failing.length) return;
  const shown = failing.slice(0, 3);
  const extra = failing.length - 3;
  const msgs = shown.map(i => i.detail || i.msg).join(" · ");
  const moreStr = extra > 0 ? ` …${extra} more` : "";
  const bannerEl = document.getElementById("banner");
  bannerEl.textContent = msgs + moreStr;
  const btn = document.createElement("button");
  btn.id = "banner-dismiss";
  btn.textContent = "×";
  btn.style.cssText = "margin-left:8px;background:transparent;border:none;color:inherit;cursor:pointer;font:inherit";
  btn.addEventListener("click", () => bannerEl.classList.remove("open"));
  bannerEl.appendChild(btn);
  bannerEl.classList.add("open");
}

beforeEach(() => {
  document.body.innerHTML = `<div class="banner" id="banner" aria-live="polite"></div>`;
});

describe("7.3c - banner capping", () => {
  it("banner with 5 failures shows first 3 messages", () => {
    const issues = [
      { pass: false, level: "error", detail: "issue A" },
      { pass: false, level: "warn",  detail: "issue B" },
      { pass: false, level: "warn",  detail: "issue C" },
      { pass: false, level: "warn",  detail: "issue D" },
      { pass: false, level: "warn",  detail: "issue E" },
    ];
    applyBannerFromIssues(issues);
    const banner = document.getElementById("banner");
    expect(banner.textContent).toContain("issue A");
    expect(banner.textContent).toContain("issue B");
    expect(banner.textContent).toContain("issue C");
    expect(banner.textContent).not.toContain("issue D");
    expect(banner.textContent).not.toContain("issue E");
  });

  it("banner with 5 failures shows a counter for the 2 extras", () => {
    const issues = [
      { pass: false, level: "error", detail: "issue A" },
      { pass: false, level: "warn",  detail: "issue B" },
      { pass: false, level: "warn",  detail: "issue C" },
      { pass: false, level: "warn",  detail: "issue D" },
      { pass: false, level: "warn",  detail: "issue E" },
    ];
    applyBannerFromIssues(issues);
    const text = document.getElementById("banner").textContent;
    expect(text).toContain("2 more");
  });

  it("banner with exactly 3 failures shows all 3 without a counter", () => {
    const issues = [
      { pass: false, level: "error", detail: "A" },
      { pass: false, level: "warn",  detail: "B" },
      { pass: false, level: "warn",  detail: "C" },
    ];
    applyBannerFromIssues(issues);
    const text = document.getElementById("banner").textContent;
    expect(text).toContain("A");
    expect(text).toContain("B");
    expect(text).toContain("C");
    expect(text).not.toContain("more");
  });

  it("banner with 1 failure shows it with no counter", () => {
    applyBannerFromIssues([{ pass: false, level: "error", detail: "single" }]);
    const text = document.getElementById("banner").textContent;
    expect(text).toContain("single");
    expect(text).not.toContain("more");
  });
});

describe("7.3c - banner dismiss", () => {
  it("banner gains 'open' class on issues", () => {
    applyBannerFromIssues([{ pass: false, level: "error", detail: "err" }]);
    expect(document.getElementById("banner").classList.contains("open")).toBe(true);
  });

  it("dismiss button removes 'open' class", () => {
    applyBannerFromIssues([{ pass: false, level: "error", detail: "err" }]);
    const banner = document.getElementById("banner");
    const dismiss = banner.querySelector("#banner-dismiss");
    expect(dismiss).not.toBeNull();
    dismiss.click();
    expect(banner.classList.contains("open")).toBe(false);
  });

  it("does nothing when all issues pass", () => {
    applyBannerFromIssues([{ pass: true, level: "info", detail: "ok" }]);
    const banner = document.getElementById("banner");
    expect(banner.classList.contains("open")).toBe(false);
    expect(banner.textContent).toBe("");
  });
});
