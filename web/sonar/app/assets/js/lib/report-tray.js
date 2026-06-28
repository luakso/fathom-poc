// Generic report tray: stores plain pin objects, composes a thread, and draws
// a 1200×675 X card. Role-agnostic — the page supplies the brand suffix and
// the artifact meta; panels supply pin objects { title, value, context, denom,
// series? }. Mirrors economy's tray.js but with no economy-specific state.
import { $, $$ } from "../dom.js";

export function createTray({ brand, meta }) {
  const pins = [];
  let sel = 0;

  function addPin(pin) {
    if (!pin) return;            // panel had nothing pinnable in this window
    pins.push(pin);
    sel = pins.length - 1;
    rTray(); renderCard();
  }

  function rTray() {
    $("#pincount").textContent = pins.length;
    $("#pinlist").innerHTML = pins.length ? pins.map((p, i) => `
      <div class="pinitem ${i === sel ? "sel" : ""}" data-i="${i}">
        <div class="t"><span>⊞ ${p.title}</span><button data-del="${i}" title="remove">✕</button></div>
        <div class="v">${p.value}</div><div class="c">${p.context}</div>
      </div>`).join("")
      : `<div class="empty">no pins yet — click ⊞ on a panel or leaderboard row</div>`;
    $$("#pinlist .pinitem").forEach(el => el.addEventListener("click", e => {
      if (e.target.dataset.del !== undefined && e.target.dataset.del !== "") return;
      sel = +el.dataset.i; rTray(); renderCard();
    }));
    $$("#pinlist [data-del]").forEach(b => b.addEventListener("click", e => {
      e.stopPropagation(); pins.splice(+b.dataset.del, 1);
      sel = Math.max(0, Math.min(sel, pins.length - 1)); rTray(); renderCard();
    }));
  }

  function genThread() {
    if (!pins.length) { $("#thread").value = ""; rCount(); return; }
    const head = `fathom // ${brand} — x402 on Base, data through ${meta.data_through_day}\n\n`;
    const body = pins.map(p => `▸ ${p.value} — ${p.context}`).join("\n");
    const foot = `\n\ndenominators stated. no row dropped — only labeled.`;
    $("#thread").value = head + body + foot; rCount();
  }
  function rCount() {
    const n = $("#thread").value.length, el = $("#charcount");
    el.textContent = n + " chars" + (n > 280 ? " · split into " + Math.ceil(n / 280) + " posts" : " · fits one post");
    el.classList.toggle("bad", n > 280);
  }

  function renderCard() {
    const cv = $("#xcard"); if (!cv || !cv.getContext) return;
    let ctx; try { ctx = cv.getContext("2d"); } catch (e) { return; }
    if (!ctx) return;
    const W = 1200, H = 675;
    ctx.fillStyle = "#070b09"; ctx.fillRect(0, 0, W, H);
    ctx.strokeStyle = "rgba(61,214,140,.05)"; ctx.lineWidth = 1;
    for (let gx = 0; gx < W; gx += 60) { ctx.beginPath(); ctx.moveTo(gx, 0); ctx.lineTo(gx, H); ctx.stroke(); }
    for (let gy = 0; gy < H; gy += 60) { ctx.beginPath(); ctx.moveTo(0, gy); ctx.lineTo(W, gy); ctx.stroke(); }
    ctx.strokeStyle = "#2a362f"; ctx.lineWidth = 2; ctx.strokeRect(24, 24, W - 48, H - 48);
    ctx.textBaseline = "alphabetic";
    ctx.fillStyle = "#3dd68c"; ctx.font = "700 26px 'JetBrains Mono', monospace"; ctx.fillText("▼ FATHOM", 64, 92);
    ctx.fillStyle = "#8fa098"; ctx.font = "400 22px 'JetBrains Mono', monospace"; ctx.fillText("::" + brand, 207, 92);
    const p = pins[sel];
    if (!p) {
      ctx.fillStyle = "#5b6a61"; ctx.font = "400 28px 'JetBrains Mono', monospace";
      ctx.fillText("pin a finding to render its card", 64, 360); return;
    }
    ctx.fillStyle = "#5b6a61"; ctx.font = "400 20px 'JetBrains Mono', monospace";
    ctx.textAlign = "right"; ctx.fillText(p.title, W - 64, 92); ctx.textAlign = "left";
    ctx.fillStyle = "#d6e2da"; ctx.font = "800 92px 'JetBrains Mono', monospace";
    if (ctx.measureText(p.value).width > W - 128) ctx.font = "800 64px 'JetBrains Mono', monospace";
    ctx.fillText(p.value, 64, 270);
    ctx.fillStyle = "#8fa098"; ctx.font = "400 26px 'JetBrains Mono', monospace";
    wrap(ctx, p.context, 64, 330, W - 128, 38);
    if (p.series && p.series.length > 1) {
      const sx = 64, sw = W - 128, sy0 = 420, sh = 130, mx = Math.max(...p.series);
      ctx.strokeStyle = "#3dd68c"; ctx.lineWidth = 2.5; ctx.beginPath();
      p.series.forEach((v, i) => { const xx = sx + sw * i / (p.series.length - 1), yy = sy0 + sh * (1 - v / mx); i ? ctx.lineTo(xx, yy) : ctx.moveTo(xx, yy); });
      ctx.stroke();
    }
    ctx.fillStyle = "#f0635a"; ctx.font = "700 17px 'JetBrains Mono', monospace"; ctx.fillText("DENOM", 64, H - 84);
    ctx.fillStyle = "#8fa098"; ctx.font = "400 17px 'JetBrains Mono', monospace"; wrap(ctx, p.denom, 150, H - 84, W - 280, 24);
    ctx.fillStyle = "#5b6a61"; ctx.font = "400 16px 'JetBrains Mono', monospace";
    ctx.fillText(`data through ${meta.data_through_day} · methodology v${meta.methodology_version} · measured, not claimed`, 64, H - 46);
  }
  function wrap(ctx, text, x, y, maxW, lh) {
    const words = String(text).split(" "); let line = "", yy = y;
    words.forEach(w => { if (ctx.measureText(line + w).width > maxW && line) { ctx.fillText(line, x, yy); line = w + " "; yy += lh; } else line += w + " "; });
    ctx.fillText(line.trim(), x, yy);
  }

  function toggle(force) {
    const t = $("#tray"); t.classList.toggle("open", force);
    if (t.classList.contains("open")) { rTray(); renderCard(); }
  }

  function init() {
    rTray();
    $("#traytoggle").addEventListener("click", () => toggle());
    $("#trayclose").addEventListener("click", () => toggle(false));
    $("#regen").addEventListener("click", genThread);
    $("#thread").addEventListener("input", rCount);
    $("#copythread").addEventListener("click", async () => {
      try { await navigator.clipboard.writeText($("#thread").value); $("#copythread").textContent = "COPIED ✓"; setTimeout(() => $("#copythread").textContent = "COPY", 1200); } catch (e) {}
    });
    $("#cardprev").addEventListener("click", () => { if (pins.length) { sel = (sel - 1 + pins.length) % pins.length; rTray(); renderCard(); } });
    $("#cardnext").addEventListener("click", () => { if (pins.length) { sel = (sel + 1) % pins.length; rTray(); renderCard(); } });
    $("#carddl").addEventListener("click", () => { const a = document.createElement("a"); a.download = `fathom-${brand}-card.png`; a.href = $("#xcard").toDataURL("image/png"); a.click(); });
  }

  return { addPin, genThread, renderCard, toggle, init };
}
