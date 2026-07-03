// Report tray: pin findings -> compose thread -> 1200x675 X card.
import { $, $$ } from "./dom.js";
import { num, fmtInt, fmtMoney, fmtCount, fmtAmt, pct, priceRead, claimVerdict, escHtml, BANDDEF } from "./format.js";
import { medianOf, peakIndex } from "./stats.js";
import { tapeSlice } from "./charts.js";
import { state, data, winLabel, facData } from "./state.js";

const pins = [];
let selPin = 0;
function overviewPinDenom() {
  const defn = "A verified payment is a USDC payment settled on Base by a known x402 facilitator.";
  const ex = data.excluded;
  if (!ex || !ex.txn_count) return defn;
  const n = ex.txn_count;
  const nStr = n >= 1e6 ? (n/1e6).toFixed(1)+"M" : n >= 1e3 ? (n/1e3).toFixed(0)+"k" : String(n);
  const v = num(ex.volume_usdc);
  const vStr = v >= 1e9 ? "$"+(v/1e9).toFixed(1)+"B" : v >= 1e6 ? "$"+(v/1e6).toFixed(0)+"M" : "$"+(v/1e3).toFixed(0)+"k";
  return `${defn} ${nStr} non-verified transfers (${vStr}) excluded.`;
}
export const PINNERS = {
  overview(){ const w = data.windows[state.win];
    const t = data.typical[state.win];
    // 6.2: append largest payment to context when the artifact carries the field.
    const largestCtx = t.largest_payment_usdc != null
      ? ` · largest ${fmtMoney(t.largest_payment_usdc)}`
      : "";
    return { title:"OVERVIEW · "+state.win.toUpperCase(), value:fmtMoney(w.volume_usdc),
      context:`median ${fmtAmt(t.median_usdc)} typical · avg ${fmtAmt(t.avg_usdc)} pulled up by large payments · ${fmtCount(w.txn_count)} verified payments${largestCtx}`,
      denom:overviewPinDenom()+" · "+winLabel[state.win],
      series:tapeSlice(data.daily, state.win).map(d=>d[2]) }; },
  daily(){ const slice = tapeSlice(data.daily, state.dWin);
    if (!slice.length) return null;
    const usd = state.dMetric === "usd";
    // Exclude incomplete points (d[3] === false) from peak selection.
    const complete = slice.filter(d => d[3] !== false);
    const peakPool = complete.length ? complete : slice; // fall back if all incomplete
    const peak = usd ? peakPool.reduce((a,b)=> b[2]>a[2]?b:a) : peakPool.reduce((a,b)=> b[1]>a[1]?b:a);
    const val  = usd ? fmtMoney(peak[2])+" vol/day peak" : fmtInt(peak[1])+" tx/day peak";
    const hasPartial = slice.some(d => d[3] === false);
    const partialNote = hasPartial ? " · last day partial, excluded from peak" : "";
    return { title:"DAILY TAPE · "+state.dWin.toUpperCase(), value:val,
      context:`${peak[0]} · ${state.dMa==="ma7"?"7-day MA":"raw"} · ${state.dScale}`,
      denom:`verified payments, ${slice[0][0]} → ${slice[slice.length-1][0]}${partialNote}`,
      series:slice.map(d=> usd?d[2]:d[1]) }; },
  monthly(){
    const complete = data.monthly.filter(m => m.complete);
    if (complete.length < 2) return { title:"MONTHLY", value:"insufficient complete months",
      context:"need two complete months for MoM", denom:"verified payments · complete months",
      series: data.monthly.map(m => num(m.volume_usdc)) };
    const prev = complete[complete.length-2], last = complete[complete.length-1];
    const v = m => num(m.volume_usdc);
    const t = m => m.txn_count;
    const dUsd = 100*(v(last)-v(prev))/v(prev);
    const dTx  = 100*(t(last)-t(prev))/t(prev);
    const name = new Date(last.month + "-01T00:00:00Z").toLocaleString("en-US",{month:"short", timeZone:"UTC"});
    return { title:"MONTHLY",
      value:`${name} $: ${dUsd>0?"+":""}${dUsd.toFixed(0)}% MoM`,
      context:`tx ${dTx>0?"+":""}${dTx.toFixed(0)}% while $ ${dUsd<0?"fell":"rose"} ${fmtMoney(v(prev))}→${fmtMoney(v(last))}`,
      denom:"verified payments · complete months",
      series: data.monthly.map(m => num(m.volume_usdc)) };
  },
  shape(){ const t = data.typical[state.win];
    if (!t.txn_count) return null;
    const b = data.windows[state.win].by_band;
    const totTx  = BANDDEF.reduce((s,[k]) => s + b[k].txn_count, 0) || 1;
    const totUsd = BANDDEF.reduce((s,[k]) => s + num(b[k].volume_usdc), 0) || 1;
    const dustTxPct  = (100 * b.dust.txn_count / totTx).toFixed(1);
    const dustUsdPct = (100 * num(b.dust.volume_usdc) / totUsd).toFixed(1);
    const xMed = num(t.avg_usdc)/num(t.median_usdc);
    // 6.3 percentile context: appended when artifact carries p10/p90/p99.
    const pctCtx = t.p10_usdc != null && t.p90_usdc != null && t.p99_usdc != null
      ? ` · p10 ${fmtAmt(t.p10_usdc)} → p90 ${fmtAmt(t.p90_usdc)} → p99 ${fmtAmt(t.p99_usdc)}`
      : "";
    return { title:"PAYMENT SHAPE · "+state.win.toUpperCase(), value:fmtAmt(t.median_usdc)+" median",
      context:`mean ${fmtAmt(t.avg_usdc)} = ${isFinite(xMed) ? Math.round(xMed).toLocaleString() : "—"}× median · dust: ${dustTxPct}% of payments, ${dustUsdPct}% of dollars${pctCtx}`,
      denom:"verified payments · "+winLabel[state.win] }; },
  price(){ const p = data.price_points[state.win][0];
    if (!p) return null;
    const READ = { menu:"a menu, not a market", market:"a market, not a menu", mixed:"between menu and market" };
    // 6.7: append payee trend for the top price point when breadth data is available.
    // Compares first vs last complete-day payee count with a ±20% threshold.
    let trendCtx = "";
    if (data.price_breadth){
      const bs = data.price_breadth.find(b => b.amount_usdc === p.amount_usdc);
      if (bs && bs.series){
        const complete = bs.series.filter(d => d.complete !== false);
        if (complete.length >= 2){
          const first = complete[0].payee_count, last = complete[complete.length-1].payee_count;
          const change = first > 0 ? (last - first) / first : null;
          if (change !== null){
            const trend = change >= 0.2 ? "widening" : change <= -0.2 ? "narrowing" : "flat";
            trendCtx = ` · payees trend: ${trend}`;
          }
        }
      }
    }
    return { title:"PRICE POINTS · "+state.win.toUpperCase(), value:fmtAmt(p.amount_usdc)+" × "+fmtCount(p.txn_count),
      context:`top amount = ${num(p.txn_share_pct).toFixed(1)}% of verified tx across ${fmtInt(p.payee_count)} payees — ${READ[priceRead(p)]}${trendCtx}`,
      denom:"verified payments · "+winLabel[state.win] }; },
  gas(){ const g = data.gas.windows[state.win];
    if (!g.txn_count) return null;
    const p = 100*g.breakeven_txn_count/g.txn_count;
    return { title:"GAS / BREAKEVEN · "+state.win.toUpperCase(), value:p.toFixed(1)+"% cost>value",
      context:`${fmtInt(g.breakeven_txn_count)} of ${fmtInt(g.txn_count)} verified payments cost more than value · ${g.gas_cents_per_dollar === null ? "—" : num(g.gas_cents_per_dollar).toFixed(2)+"¢"} true cost (L1+L2) per $1`,
      denom:"tx-deduped L1+L2 cost, equal apportioning · monthly ETH/USD ref · "+winLabel[state.win] }; },
  velocity(){ const vw = data.velocity.windows.all;
    const days = data.velocity.verified_daily;
    if (!days.length) return null;
    const pi = peakIndex(days.map(d => d[1]));
    const med = medianOf(days.map(d => d[2]));
    return { title:"VELOCITY", value:fmtInt(vw.max_per_min)+"/min peak",
      context:`${days[pi][0]} · body ~${fmtInt(med)}/min p99 — bursts, not drip`,
      denom:"verified payments · p99 over active minutes",
      series: days.map(d => d[1]) }; },
  claims(){ const c = data.claims[0];
    if (!c) return null;
    const measured = num(c.measured_value);
    const ratio = measured !== 0 ? num(c.claimed_value) / measured : null;
    const isUsd = (c.measured_unit || "").toUpperCase() === "USDC";
    const fmt = isUsd ? fmtMoney : fmtInt;
    return { title:"CLAIM LEDGER", value:"claim "+claimVerdict(ratio),
      context:`"${escHtml(c.claim_text)}" → measured ${fmt(c.measured_value)}`,
      denom:"claim vs Fathom measured count" }; },
  facilitators(){
    if (!facData || !facData.rows || !facData.rows.length) return null;
    const r = facData.rows[0];
    const hasWindows = !!(r.windows && r.windows["7d"] && r.windows["30d"]);
    if (!hasWindows) return null;
    const shortAddr = a => String(a).length > 10 ? String(a).slice(0,6) + "…" + String(a).slice(-4) : a;
    const v7  = num(r.windows["7d"].volume_usdc);
    const v30 = num(r.windows["30d"].volume_usdc);
    const momentum = v30 > 0 ? (100 * v7 / v30).toFixed(0) + "%" : "—";
    const topN = facData.rows.slice(0, 3).map(x => `${shortAddr(x.facilitator)} ${fmtMoney(x.volume_usdc)}`).join(" · ");
    return { title:"FACILITATORS",
      value:`${shortAddr(r.facilitator)} · ${fmtMoney(r.volume_usdc)} all-time`,
      context:`top facilitators: ${topN} · 7d momentum ${momentum} of 30d`,
      denom:"who settled the payments · verified payments only · momentum = last 7 days' share of the last 30" }; },
  active_wallets(){ const ae = data.active_entities;
    if (!ae || !ae.length) return null;
    // Show counts from the last complete day (exclude the partial edge day).
    const complete = ae.filter(p => p.complete);
    const ref = complete.length ? complete[complete.length-1] : ae[ae.length-1];
    // 6.5: append 7d new-payer% when the artifact carries cohort data.
    const cohorts = data.payer_cohorts;
    let cohortCtx = "";
    if (cohorts && cohorts["7d"]) {
      const c = cohorts["7d"];
      const nv = num(c.new_payer_volume_usdc), rv = num(c.returning_payer_volume_usdc);
      const tot = nv + rv || 1;
      const np = (100 * nv / tot).toFixed(1);
      cohortCtx = ` · 7d: ${np}% new-payer volume`;
    }
    return { title:"ACTIVE WALLETS",
      value:`${fmtInt(ref.payer_count)} payers · ${fmtInt(ref.payee_count)} payees`,
      context:`${ref.day} · distinct wallets from verified payments${cohortCtx}`,
      denom:"distinct paying and receiving wallets per day · verified payments",
      series: ae.map(p => p.payer_count) }; },
};
export function addPin(key){
  const gen = PINNERS[key]; if (!gen) return;
  const pin = gen(); if (!pin) return; // panel has nothing pinnable in this window
  pins.push({ key, win:state.win, ...pin });
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
  const foot = `\n\nverified payments only. unverified transfers excluded. denominators stated.`;
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
  const cv = $("#xcard"); if (!cv || !cv.getContext) return;
  let ctx; try { ctx = cv.getContext("2d"); } catch(e){ return; }
  if (!ctx) return;
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
  ctx.fillText(`data through ${data.meta.data_through_day} · methodology v${data.meta.methodology_version} · measured, not claimed`, 64, H-46);
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

/** Reset pin state between tests — not used in production. */
export function _clearPins(){ pins.length = 0; selPin = 0; }

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
