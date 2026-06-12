// Report tray: pin findings -> compose thread -> 1200x675 X card.
import { $, $$ } from "./dom.js";
import { num, fmtInt, fmtMoney, fmtCount, fmtAmt, pct } from "./format.js";
import { state, data, winLabel } from "./state.js";

const pins = [];
let selPin = 0;
const PINNERS = {
  overview(){ const w = data.windows[state.win], a = w.by_attribution;
    return { title:"OVERVIEW · "+state.win.toUpperCase(), value:fmtMoney(a.agentic.volume_usdc)+" agentic",
      context:`of ${fmtMoney(w.volume_usdc)} observed (${pct(a.agentic.volume_usdc,w.volume_usdc)} of $, ${pct(a.agentic.txn_count,w.txn_count)} of tx)`,
      denom:"all observed x402-surface USDC on Base · "+winLabel[state.win],
      series:data.daily.map(d=>d[1]) }; },
  daily(){ const peak = data.daily.reduce((a,b)=> b[1]>a[1]?b:a);
    return { title:"DAILY TAPE", value:fmtInt(peak[1])+" tx/day peak",
      context:`${peak[0]} · ${state.dMa==="ma7"?"7-day MA":"raw"} · ${state.dScale}`,
      denom:`all observed traffic, ${data.daily[0][0]} → ${data.meta.data_through_day}`,
      series:data.daily.map(d=> state.dMetric==="tx"?d[1]:d[2]) }; },
  monthly(){
    const complete = data.monthly.filter(m => m.complete);
    if (complete.length < 2) return { title:"MONTHLY · AGENTIC", value:"insufficient complete months",
      context:"need two complete months for MoM", denom:"agentic set only · complete months",
      series: data.monthly.map(m => num(m.by_attribution.agentic.volume_usdc)) };
    const prev = complete[complete.length-2], last = complete[complete.length-1];
    const v = m => num(m.by_attribution.agentic.volume_usdc);
    const t = m => m.by_attribution.agentic.txn_count;
    const dUsd = 100*(v(last)-v(prev))/v(prev);
    const dTx  = 100*(t(last)-t(prev))/t(prev);
    const name = new Date(last.month + "-01T00:00:00Z").toLocaleString("en-US",{month:"short", timeZone:"UTC"});
    return { title:"MONTHLY · AGENTIC",
      value:`${name} $: ${dUsd>0?"+":""}${dUsd.toFixed(0)}% MoM`,
      context:`tx ${dTx>0?"+":""}${dTx.toFixed(0)}% while $ ${dUsd<0?"fell":"rose"} ${fmtMoney(v(prev))}→${fmtMoney(v(last))}${dUsd<0&&dTx>0?" — activity without value":""}`,
      denom:"agentic set only · complete months",
      series: data.monthly.map(m => num(m.by_attribution.agentic.volume_usdc)) };
  },
  split(){ const w = data.windows[state.win], a = w.by_attribution;
    return { title:"ATTRIBUTION · "+state.win.toUpperCase(), value:pct(a.contamination.volume_usdc,w.volume_usdc)+" contamination",
      context:`agentic ${fmtMoney(a.agentic.volume_usdc)} · contested ${fmtMoney(a.contested.volume_usdc)} · contamination ${fmtMoney(a.contamination.volume_usdc)}`,
      denom:"allowlist v1 — 112 facilitators · "+winLabel[state.win] }; },
  shape(){ const t = data.typical[state.win];
    return { title:"PAYMENT SHAPE · "+state.win.toUpperCase(), value:fmtAmt(t.agentic.median_usdc)+" median",
      context:`mean ${fmtAmt(t.agentic.avg_usdc)} = ${Math.round(num(t.agentic.avg_usdc)/num(t.agentic.median_usdc)).toLocaleString()}× median · contamination median ${fmtAmt(t.contamination.median_usdc)}`,
      denom:"agentic set · "+winLabel[state.win] }; },
  price(){ const p = data.price_points[state.win][0];
    return { title:"PRICE POINTS · "+state.win.toUpperCase(), value:fmtAmt(p.amount_usdc)+" × "+fmtCount(p.txn_count),
      context:`top amount = ${p.txn_share_pct}% of agentic tx across ${fmtInt(p.payee_count)} payees — a menu, not a market`,
      denom:"agentic set only · "+winLabel[state.win] }; },
  gas(){ const g = data.gas.windows[state.win].by_attribution.agentic;
    const p = 100*g.breakeven_txn_count/g.txn_count;
    return { title:"GAS / BREAKEVEN · "+state.win.toUpperCase(), value:p.toFixed(1)+"% gas>value",
      context:`${fmtInt(g.breakeven_txn_count)} of ${fmtInt(g.txn_count)} agentic payments · ${g.gas_cents_per_dollar === null ? "—" : num(g.gas_cents_per_dollar).toFixed(2)+"¢"} gas per $1 settled`,
      denom:"tx-deduped gas, equal apportioning · monthly ETH/USD ref · "+winLabel[state.win] }; },
  velocity(){ const vw = data.velocity.windows.all.agentic;
    const days = data.velocity.agentic_daily;
    let pi = 0; days.forEach((d,i) => { if (d[1] > days[pi][1]) pi = i; });
    const med = [...days.map(d => d[1])].sort((a,b) => a-b)[Math.floor(days.length/2)] ?? 0;
    return { title:"VELOCITY", value:fmtInt(vw.max_per_min)+"/min peak",
      context:`${days[pi][0]} · body ~${fmtInt(med)}/min — bursts, not drip`,
      denom:"agentic set · p99 over active minutes",
      series: days.map(d => d[1]) }; },
  claims(){ const c = data.claims[0];
    return { title:"CLAIM LEDGER", value:"×"+(num(c.claimed_value)/num(c.measured_value)).toFixed(1)+" overstated",
      context:`"${c.claim_text}" → measured ${fmtInt(c.measured_value)}`,
      denom:"claim vs Fathom classified agentic count" }; },
};
export function addPin(key){
  const gen = PINNERS[key]; if (!gen) return;
  pins.push({ key, win:state.win, ...gen() });
  selPin = pins.length-1;
  rTray(); rCard();
  $("#pincount").textContent = pins.length;
  const panel = $(`[data-pin="${key}"]`)?.closest(".panel");
  if (panel){ panel.classList.add("pinned-flash"); setTimeout(() => panel.classList.remove("pinned-flash"), 380); }
}
export function rTray(){
  $("#pincount").textContent = pins.length;
  $("#pinlist").innerHTML = pins.length ? pins.map((p,i) => `
    <div class="pinitem ${i===selPin?"sel":""}" data-i="${i}">
      <div class="t"><span>⊞ ${p.title}</span><button data-del="${i}" title="remove">✕</button></div>
      <div class="v">${p.value}</div>
      <div class="c">${p.context}</div>
    </div>`).join("")
    : `<div class="empty">no pins yet — hover a panel and press <span class="c-ac">p</span>, or click ⊞ pin</div>`;
  $$("#pinlist .pinitem").forEach(el => el.addEventListener("click", e => {
    if (e.target.dataset.del !== undefined && e.target.dataset.del !== "") return;
    selPin = +el.dataset.i; rTray(); rCard();
  }));
  $$("#pinlist [data-del]").forEach(b => b.addEventListener("click", e => {
    e.stopPropagation();
    pins.splice(+b.dataset.del,1);
    selPin = Math.max(0, Math.min(selPin, pins.length-1));
    rTray(); rCard();
  }));
}
export function genThread(){
  if (!pins.length){ $("#thread").value = ""; rCount(); return; }
  const head = `fathom // x402 on Base — data through ${data.meta.data_through_day}\n\n`;
  const body = pins.map(p => `▸ ${p.value} — ${p.context}`).join("\n");
  const foot = `\n\ndenominators stated. no row dropped — only labeled.`;
  $("#thread").value = head + body + foot;
  rCount();
}
export function rCount(){
  const n = $("#thread").value.length;
  const el = $("#charcount");
  el.textContent = n + " chars" + (n > 280 ? " · split into " + Math.ceil(n/280) + " posts" : " · fits one post");
  el.classList.toggle("bad", n > 280);
}
/* X card — canvas 1200×675 */
export function rCard(){
  const cv = $("#xcard"), ctx = cv.getContext("2d");
  const W = 1200, H = 675;
  ctx.fillStyle = "#070b09"; ctx.fillRect(0,0,W,H);
  // grid
  ctx.strokeStyle = "rgba(61,214,140,.05)"; ctx.lineWidth = 1;
  for (let gx=0; gx<W; gx+=60){ ctx.beginPath(); ctx.moveTo(gx,0); ctx.lineTo(gx,H); ctx.stroke(); }
  for (let gy=0; gy<H; gy+=60){ ctx.beginPath(); ctx.moveTo(0,gy); ctx.lineTo(W,gy); ctx.stroke(); }
  // scanlines
  ctx.fillStyle = "rgba(0,0,0,.16)";
  for (let sy=0; sy<H; sy+=3) ctx.fillRect(0,sy,W,1);
  // frame
  ctx.strokeStyle = "#2a362f"; ctx.lineWidth = 2; ctx.strokeRect(24,24,W-48,H-48);
  const p = pins[selPin];
  ctx.textBaseline = "alphabetic";
  // brand
  ctx.fillStyle = "#3dd68c"; ctx.font = "700 26px 'JetBrains Mono', monospace";
  ctx.fillText("▼ FATHOM", 64, 92);
  ctx.fillStyle = "#8fa098"; ctx.font = "400 22px 'JetBrains Mono', monospace";
  ctx.fillText("::economy", 207, 92);
  if (!p){
    ctx.fillStyle = "#5b6a61"; ctx.font = "400 28px 'JetBrains Mono', monospace";
    ctx.fillText("pin a finding to render its card", 64, 360);
    return;
  }
  ctx.fillStyle = "#5b6a61"; ctx.font = "400 20px 'JetBrains Mono', monospace";
  ctx.textAlign = "right"; ctx.fillText(p.title, W-64, 92); ctx.textAlign = "left";
  // big value
  ctx.fillStyle = "#d6e2da"; ctx.font = "800 92px 'JetBrains Mono', monospace";
  let val = p.value;
  if (ctx.measureText(val).width > W-128){ ctx.font = "800 64px 'JetBrains Mono', monospace"; }
  ctx.fillText(val, 64, 270);
  // context
  ctx.fillStyle = "#8fa098"; ctx.font = "400 26px 'JetBrains Mono', monospace";
  wrapText(ctx, p.context, 64, 330, W-128, 38);
  // sparkline
  if (p.series && p.series.length > 1){
    const sx = 64, sw = W-128, sy0 = 420, sh = 130;
    const mx = Math.max(...p.series);
    ctx.strokeStyle = "#3dd68c"; ctx.lineWidth = 2.5; ctx.beginPath();
    p.series.forEach((v,i) => {
      const xx = sx + sw*i/(p.series.length-1), yy = sy0 + sh*(1 - v/mx);
      i ? ctx.lineTo(xx,yy) : ctx.moveTo(xx,yy);
    });
    ctx.stroke();
    ctx.strokeStyle = "#1d2722"; ctx.beginPath(); ctx.moveTo(sx, sy0+sh+8); ctx.lineTo(sx+sw, sy0+sh+8); ctx.stroke();
  }
  // denominator + stamp
  ctx.fillStyle = "#f0635a"; ctx.font = "700 17px 'JetBrains Mono', monospace";
  ctx.fillText("DENOM", 64, H-84);
  ctx.fillStyle = "#8fa098"; ctx.font = "400 17px 'JetBrains Mono', monospace";
  wrapText(ctx, p.denom, 150, H-84, W-280, 24);
  ctx.fillStyle = "#5b6a61"; ctx.font = "400 16px 'JetBrains Mono', monospace";
  ctx.fillText("data through " + data.meta.data_through_day + " · methodology v1 · measured, not claimed", 64, H-46);
}
function wrapText(ctx, text, x, y, maxW, lh){
  const words = String(text).split(" ");
  let line = "", yy = y;
  words.forEach(w => {
    if (ctx.measureText(line + w).width > maxW && line){ ctx.fillText(line, x, yy); line = w + " "; yy += lh; }
    else line += w + " ";
  });
  ctx.fillText(line.trim(), x, yy);
}

const tray = () => $("#tray");
export function toggleTray(force){
  tray().classList.toggle("open", force);
  if (tray().classList.contains("open")){ rTray(); rCard(); }
}
export function initTray(){
  $("#traytoggle").addEventListener("click", () => toggleTray());
  $("#trayclose").addEventListener("click", () => toggleTray(false));
  $("#regen").addEventListener("click", genThread);
  $("#thread").addEventListener("input", rCount);
  $("#copythread").addEventListener("click", async () => {
    try { await navigator.clipboard.writeText($("#thread").value); $("#copythread").textContent = "COPIED ✓";
      setTimeout(() => $("#copythread").textContent = "COPY", 1200); } catch(e){}
  });
  $("#cardprev").addEventListener("click", () => { if (pins.length){ selPin = (selPin-1+pins.length)%pins.length; rTray(); rCard(); }});
  $("#cardnext").addEventListener("click", () => { if (pins.length){ selPin = (selPin+1)%pins.length; rTray(); rCard(); }});
  $("#carddl").addEventListener("click", () => {
    const a = document.createElement("a");
    a.download = "fathom-card.png";
    a.href = $("#xcard").toDataURL("image/png");
    a.click();
  });
}
