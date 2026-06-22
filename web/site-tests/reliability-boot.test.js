import { describe, it, expect, beforeEach, vi } from "vitest";
import { readFileSync, existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const distRel = resolve(here, "../../dist/reliability.json");
const distEco = resolve(here, "../../dist/economy.json");
const htmlPath = resolve(here, "../site/reliability.html");

const haveArtifacts = existsSync(distRel) && existsSync(distEco);

describe.runIf(haveArtifacts)("reliability page boots against real dist artifacts", () => {
  beforeEach(() => {
    const html = readFileSync(htmlPath, "utf8");
    const body = html.replace(/[\s\S]*<body>/, "").replace(/<\/body>[\s\S]*/, "");
    document.body.innerHTML = body;
    const files = {
      "./reliability.json": readFileSync(distRel, "utf8"),
      "./economy.json": readFileSync(distEco, "utf8"),
    };
    vi.stubGlobal("fetch", (url) => {
      const text = files[url];
      if (text == null) return Promise.resolve({ ok: false, status: 404 });
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(JSON.parse(text)) });
    });
    vi.stubGlobal("localStorage", { getItem: () => "1", setItem: () => {} });
  });

  it("reaches conservation ✓ with all panels populated", async () => {
    await import("../site/assets/js/reliability/app.js");
    for (let i = 0; i < 50 && !document.querySelector("#shell").textContent; i++) {
      await new Promise(r => setTimeout(r, 0));
    }
    expect(document.querySelector("#st-cons").textContent).toBe("conservation ✓");
    expect(document.querySelector("#fatal").classList.contains("open")).toBe(false);
    expect(document.querySelector("#ov-stats").textContent).toMatch(/SETTLEMENTS/);
    expect(document.querySelector("#lat-caveat").textContent).toMatch(/validAfter/);
    expect(document.querySelector("#st-through").textContent).toBe("2026-06-06");
  });
});
