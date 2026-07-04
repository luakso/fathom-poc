import { describe, it, expect, beforeEach, vi } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const distEco = resolve(here, "../../../dist/economy.json");
const distFac = resolve(here, "../../../dist/facilitators.json");
const htmlPath = resolve(here, "../app/index.html");

const haveArtifacts = existsSync(distEco);

async function settle(sel) {
  for (let i = 0; i < 80 && !document.querySelector(sel); i++) {
    await new Promise(r => setTimeout(r, 0));
  }
}

// Stub fetch to serve real artifacts from disk. facilitators.json served if
// present, otherwise 404 (panel 10 renders absent-state, still counts as
// non-empty since it has static skeleton HTML).
function stubFetch() {
  const files = { "./economy.json": readFileSync(distEco, "utf8") };
  if (existsSync(distFac)) {
    files["./facilitators.json"] = readFileSync(distFac, "utf8");
  }
  vi.stubGlobal("fetch", (url) => {
    const text = files[url];
    if (text == null) return Promise.resolve({ ok: false, status: 404 });
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(JSON.parse(text)) });
  });
  vi.stubGlobal("localStorage", { getItem: () => "1", setItem: () => {} });
}

function mountHtml() {
  const html = readFileSync(htmlPath, "utf8");
  const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\S]*/, "");
  document.body.innerHTML = body;
}

// ---------------------------------------------------------------------------
// Existing tape test (Stage 1 item)
// ---------------------------------------------------------------------------
describe.runIf(haveArtifacts)("economy page daily tape", () => {
  beforeEach(() => {
    mountHtml();
    stubFetch();
  });

  it("defaults to the 30D tape, re-slices on toggle, and never shades", async () => {
    await import("../app/assets/js/app.js");
    await settle("#dsvg");

    // default timeframe is 30D
    const on = document.querySelector('.ptools button[data-t="d-win"].on');
    expect(on?.dataset.v).toBe("30d");

    // the range chip reflects what's drawn
    const range30 = document.querySelector("#d-range").textContent;
    expect(range30).toMatch(/→/);

    // toggling to ALL widens the span; 7D narrows it
    document.querySelector('.ptools button[data-t="d-win"][data-v="all"]').click();
    const rangeAll = document.querySelector("#d-range").textContent;
    document.querySelector('.ptools button[data-t="d-win"][data-v="7d"]').click();
    const range7 = document.querySelector("#d-range").textContent;
    expect(rangeAll).not.toBe(range30);
    expect(range7).not.toBe(rangeAll);

    // the old "active window shaded" rect must be gone in every mode
    expect(document.querySelector("#dsvg").innerHTML).not.toMatch(/stroke-dasharray="3 3"/);
  });
});

// ---------------------------------------------------------------------------
// Item 8.5 — Real boot assertions after loading the real artifact.
// Uses vi.resetModules() to get a fresh boot on each test in this describe,
// so each test runs the IIFE against its own clean DOM.
// ---------------------------------------------------------------------------
describe.runIf(haveArtifacts)("economy page boot assertions (8.5)", () => {
  beforeEach(() => {
    vi.resetModules();
    mountHtml();
    stubFetch();
  });

  // CORE numbered panels (1 overview .. 8 claim ledger) are present in any emit,
  // stale or fresh — always assert real non-empty content. claims.json is
  // curated and always present, so #p9 (CLAIM LEDGER) belongs here too.
  const CORE_PANELS = ["p1","p2","p3","p5","p6","p7","p8","p9"];

  it("(a) CORE panels (1-8) have non-empty innerHTML after boot", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell"); // wait until verify log is populated

    for (const id of CORE_PANELS) {
      const panel = document.getElementById(id);
      expect(panel, `panel #${id} exists`).not.toBeNull();
      expect(panel.innerHTML.trim().length, `panel #${id} innerHTML non-empty`).toBeGreaterThan(0);
    }
  });

  // Panel "9 ACTIVE WALLETS" (#p11) depends on active_entities (chart) and
  // payer_cohorts (#ae-cohort sub-widget) — stage-6 sections that may be
  // absent until the background rollup+emit lands. Present-gate: real
  // content when the section exists, explicit absent-state otherwise —
  // either way it must not crash and must not be blank.
  it("(a) panel 9 ACTIVE WALLETS renders content or explicit absent-state, never blank", async () => {
    const doc = JSON.parse(readFileSync(distEco, "utf8"));
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const panel = document.getElementById("p11");
    expect(panel).not.toBeNull();
    expect(panel.innerHTML.trim().length, "p11 innerHTML non-empty").toBeGreaterThan(0);

    const cohortHtml = document.getElementById("ae-cohort").innerHTML.trim();
    if (doc.data.payer_cohorts) {
      expect(cohortHtml.length, "ae-cohort renders cohort bars when payer_cohorts present").toBeGreaterThan(0);
    } else {
      // rPayerCohorts renders "" (empty string) when payer_cohorts is absent —
      // documented old-artifact tolerance in panels.js, not a crash.
      expect(cohortHtml).toBe("");
    }
  });

  // Panel "10 FACILITATORS" (#p12) depends on the secondary facilitators.json
  // fetch and its per-row 7d/30d windows (stage-6). Present-gate: real table
  // when present, explicit absent-state text otherwise.
  it("(a) panel 10 FACILITATORS renders content or explicit absent-state, never blank", async () => {
    const haveFac = existsSync(distFac);
    const facDoc = haveFac ? JSON.parse(readFileSync(distFac, "utf8")) : null;
    const hasWindows = !!(haveFac && facDoc.data?.rows?.[0]?.windows?.["7d"] && facDoc.data.rows[0].windows["30d"]);

    await import("../app/assets/js/app.js");
    await settle("#shell");

    const panel = document.getElementById("p12");
    expect(panel).not.toBeNull();
    expect(panel.innerHTML.trim().length, "p12 innerHTML non-empty").toBeGreaterThan(0);

    const facBody = document.getElementById("fac-body").innerHTML;
    if (hasWindows) {
      expect(facBody, "fac-body renders a table when windows present").toContain("<table");
    } else {
      expect(
        facBody.includes("facilitator data unavailable") || facBody.includes("facilitator window data absent"),
        "fac-body renders explicit absent-state when windows/facilitators.json absent"
      ).toBe(true);
    }
  });

  it("(b) status stamps #st-through and #st-meth are populated", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const stThrough = document.getElementById("st-through");
    const stMeth    = document.getElementById("st-meth");

    expect(stThrough).not.toBeNull();
    expect(stMeth).not.toBeNull();

    // data_through_day is a YYYY-MM-DD date
    expect(stThrough.textContent).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    // methodology version stamp is "v<N>"
    expect(stMeth.textContent).toMatch(/^v\d+$/);
  });

  it("(b) conservation glyph in #st-cons shows ✓ or ✗ (populated)", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const stCons = document.getElementById("st-cons");
    expect(stCons).not.toBeNull();
    const text = stCons.textContent;
    // Must show one of the two states (populated, not "loading" placeholder)
    expect(text === "conservation ✓" || text === "conservation ✗").toBe(true);
  });

  it("(b) conservation glyph shows ✓ for a clean, verified artifact", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const stCons = document.getElementById("st-cons");
    // The real artifact passes conservation — report if it doesn't (data quality issue)
    expect(stCons.textContent).toBe("conservation ✓");
  });

  it("(c) no banner open after clean boot", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const banner = document.getElementById("banner");
    expect(banner).not.toBeNull();
    expect(banner.classList.contains("open")).toBe(false);
  });

  it("(d) verify-log (#shell) renders check lines with ✓ glyphs", async () => {
    await import("../app/assets/js/app.js");
    await settle("#shell");

    const shell = document.getElementById("shell");
    expect(shell).not.toBeNull();
    const html = shell.innerHTML;
    // Shell must contain the conservation output line with OK glyph
    expect(html).toContain("✓");
    // Shell must reference the methodology version and scope
    expect(html).toContain("verified-x402");
    // Every check line must be a passing check (class "ok"), no failing checks
    const failSpans = shell.querySelectorAll("span.fail");
    expect(failSpans.length).toBe(0);
  });
});
