import { describe, it, expect, beforeEach, vi } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const distEco = resolve(here, "../../dist/economy.json");
const htmlPath = resolve(here, "../site/index.html");

const haveArtifacts = existsSync(distEco);

async function settle(sel) {
  for (let i = 0; i < 80 && !document.querySelector(sel); i++) {
    await new Promise(r => setTimeout(r, 0));
  }
}

describe.runIf(haveArtifacts)("economy page daily tape", () => {
  beforeEach(() => {
    const html = readFileSync(htmlPath, "utf8");
    const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\S]*/, "");
    document.body.innerHTML = body;
    const files = { "./economy.json": readFileSync(distEco, "utf8") };
    vi.stubGlobal("fetch", (url) => {
      const text = files[url];
      if (text == null) return Promise.resolve({ ok: false, status: 404 });
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(JSON.parse(text)) });
    });
    vi.stubGlobal("localStorage", { getItem: () => "1", setItem: () => {} });
  });

  it("defaults to the 30D tape, re-slices on toggle, and never shades", async () => {
    await import("../site/assets/js/app.js");
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
