// Chart-heavy renderers: daily tape, monthly bars, velocity, active wallets.
// Hand-rolled SVG; innerHTML replacement on each render means hover listeners
// die with their nodes (no leak).
import { $ } from "./dom.js";
import { num, fmtInt, fmtUSDC, fmtUSDCFull, fmtCompact, fmtCount } from "./format.js";
import { medianOf, peakIndex } from "./stats.js";
import { state, data } from "./state.js";

function ma7(arr){ return arr.map((_,i) => { const s = arr.slice(Math.max(0,i-6), i+1); return s.reduce((x,y)=>x+y,0)/s.length; }); }

// hasIncomplete: true when the last element of the tuple array is incomplete (d[3] === false).
function hasIncomplete(days){ return days.length > 0 && days[days.length-1][3] === false; }

// tapeSlice: the daily tape shows ONLY the selected timeframe (not the full
// series with the window shaded). "all" is the whole series; otherwise take the
// trailing N days, clamped so a window wider than the data returns everything.
export function tapeSlice(days, win){
  if (win === "all") return days;
  const n = win === "7d" ? 7 : win === "30d" ? 30 : win === "90d" ? 90 : days.length;
  return days.slice(Math.max(0, days.length - n));
}

const monDay = s => new Date(s+"T00:00:00Z").toLocaleString("en-US",{month:"short",day:"2-digit",timeZone:"UTC"});

export function rDaily(){
  const days = tapeSlice(data.daily, state.dWin);
  const host = $("#dailychart");
  if (!days.length){
    host.innerHTML = `<div class="readout">no verified payments in this window</div>`;
    if ($("#d-range")) $("#d-range").textContent = "—";
    const partialNote = $("#d-partial-note");
    if (partialNote) partialNote.style.display = "none";
    return;
  }
  const W = host.clientWidth || 900, H = 250, padL = 56, padB = 22, padT = 12;
  // incomplete: the last point may be a partial day — exclude from MA7 and
  // from peak annotation, show visually distinct.
  const edgeIncomplete = hasIncomplete(days);
  const rawVals = days.map(d => state.dMetric === "tx" ? d[1] : d[2]);
  let vals;
  if (state.dMa === "ma7") {
    // Compute MA7 on all but the incomplete edge (if present), then keep the
    // edge at its raw value so it doesn't distort the trailing average.
    const completeCount = edgeIncomplete ? rawVals.length - 1 : rawVals.length;
    const maOnComplete = ma7(rawVals.slice(0, completeCount));
    vals = edgeIncomplete ? [...maOnComplete, rawVals[rawVals.length-1]] : maOnComplete;
  } else {
    vals = rawVals;
  }
  const positives = vals.filter(v => v > 0);
  const log = state.dScale === "log" && positives.length > 0;
  const vmax = Math.max(...vals), vmin = log ? Math.min(...positives) : 0;
  const ty = v => log ? Math.log10(Math.max(v, vmin)) : v;
  const yMax = ty(vmax), yMin = log ? ty(vmin) : 0;
  const x = i => padL + (W-padL-10) * (days.length > 1 ? i/(days.length-1) : 0.5);
  const y = v => padT + (H-padT-padB) * (1 - (ty(v)-yMin)/(yMax-yMin || 1));
  // step path
  let p = "";
  vals.forEach((v,i) => { const xx = x(i), yy = y(v);
    p += i===0 ? `M${xx},${yy}` : `H${xx}V${yy}`; });
  const area = p + `V${H-padB}H${x(0)}Z`;
  // gridlines
  let grid = "";
  let gv;
  if (log) {
    const decades = [3,4,5,6,7].map(e => Math.pow(10,e)).filter(v => v >= vmin && v <= vmax);
    if (decades.length >= 2) {
      gv = decades;
    } else {
      // Fewer than 2 decade lines visible — fall back to geometric min/mid/max.
      const logMid = Math.pow(10, (Math.log10(vmin) + Math.log10(vmax)) / 2);
      gv = [vmin, logMid, vmax];
    }
  } else {
    gv = [1,2,3].map(k => vmax*k/4);
  }
  gv.forEach(v => { grid += `<line class="gl" x1="${padL}" y1="${y(v)}" x2="${W-10}" y2="${y(v)}"/>
    <text x="${padL-7}" y="${y(v)+3}" text-anchor="end">${state.dMetric==="tx"?fmtCount(v):fmtCompact(v)}</text>`; });
  // x-axis ticks adapt to the selected slice: month markers for long spans,
  // evenly spaced dated markers for short windows (where no "-01" may fall).
  let ticks = "";
  if (days.length > 60){
    days.forEach((d,i) => { if (d[0].endsWith("-01")) {
      const lab = new Date(d[0]+"T00:00:00").toLocaleString("en-US",{month:"short"}).toUpperCase();
      ticks += `<line class="gl" x1="${x(i)}" y1="${padT}" x2="${x(i)}" y2="${H-padB}"/><text x="${x(i)+4}" y="${H-7}">${lab}</text>`; }});
  } else {
    const step = Math.max(1, Math.round(days.length/6));
    for (let i=0; i<days.length; i+=step){
      // labels read left-to-right except the right-edge tick, which anchors end
      // so it never clips past the viewBox
      const edge = i >= days.length - step;
      const tx = edge ? `x="${x(i)-4}" text-anchor="end"` : `x="${x(i)+4}"`;
      ticks += `<line class="gl" x1="${x(i)}" y1="${padT}" x2="${x(i)}" y2="${H-padB}"/><text ${tx} y="${H-7}">${monDay(days[i][0]).toUpperCase()}</text>`;
    }
  }
  // the range chip reflects exactly what's drawn
  if ($("#d-range")) $("#d-range").textContent = `${monDay(days[0][0])} → ${monDay(days[days.length-1][0])}`;
  // incomplete edge: show a hollow circle marker distinct from the main path
  const edgeMarker = edgeIncomplete
    ? `<circle data-partial="true" cx="${x(days.length-1)}" cy="${y(vals[vals.length-1])}" r="4" fill="none" stroke="var(--faint)" stroke-width="1.2" stroke-dasharray="2 2" opacity="0.6"/>`
    : "";
  // update the partial-day note visibility (data-driven, not wall-clock)
  const partialNote = $("#d-partial-note");
  if (partialNote) partialNote.style.display = edgeIncomplete ? "" : "none";
  host.innerHTML = `<svg class="ch" viewBox="0 0 ${W} ${H}" height="${H}" id="dsvg">
    ${grid}${ticks}
    <path d="${area}" fill="rgba(61,214,140,.10)"/>
    <path d="${p}" fill="none" stroke="var(--agentic)" stroke-width="1.4"/>
    ${edgeMarker}
    <line id="dcross" x1="0" x2="0" y1="${padT}" y2="${H-padB}" stroke="var(--accent)" stroke-width="1" opacity="0"/>
    <circle id="ddot" r="3" fill="var(--accent)" opacity="0"/>
    <line class="ax" x1="${padL}" y1="${H-padB}" x2="${W-10}" y2="${H-padB}"/>
    <rect id="dhover" x="${padL}" y="${padT}" width="${W-padL-10}" height="${H-padT-padB}" fill="transparent"/>
  </svg>`;
  const svg = $("#dsvg");
  $("#dhover").addEventListener("mousemove", e => {
    const r = svg.getBoundingClientRect();
    const sx = (e.clientX - r.left) * (W/r.width);
    const i = Math.max(0, Math.min(days.length-1, Math.round((sx-padL)/((W-padL-10)/(days.length-1)))));
    const d = days[i];
    $("#dcross").setAttribute("x1", x(i)); $("#dcross").setAttribute("x2", x(i)); $("#dcross").setAttribute("opacity",".7");
    $("#ddot").setAttribute("cx", x(i)); $("#ddot").setAttribute("cy", y(vals[i])); $("#ddot").setAttribute("opacity","1");
    $("#d-readout").innerHTML = `<span class="d">${d[0]}</span> ▸ ${fmtInt(d[1])} tx ▸ ${fmtUSDCFull(d[2])}${state.dMa==="ma7" ? ` ▸ ma7 ${state.dMetric==="tx"?fmtCount(vals[i]):fmtUSDC(vals[i])}`:""}`;
  });
  $("#dhover").addEventListener("mouseleave", () => {
    $("#dcross").setAttribute("opacity","0"); $("#ddot").setAttribute("opacity","0");
    $("#d-readout").textContent = "hover the tape ▸ crosshair";
  });
}

/* ———————— 3 MONTHLY ———————— */
export function rMonthly(){
  const ms = data.monthly;
  const usd = state.mMetric === "usd";
  const get = m => usd ? num(m.volume_usdc) : m.txn_count;
  const max = Math.max(...ms.map(get)) || 1;
  $("#monthly").innerHTML = ms.map((m,i) => {
    const v = get(m), wpct = Math.max(1, 72*v/max);
    let mom = `<span class="mom na">—</span>`;
    if (i>0 && m.complete && ms[i-1].complete && get(ms[i-1]) > 0){
      const d = 100*(v-get(ms[i-1]))/get(ms[i-1]);
      mom = `<span class="mom ${d<0?"dn":"up"}">${d>0?"+":""}${d.toFixed(0)}%</span>`;
    }
    if (!m.complete) mom = `<span class="mom na">◌ partial</span>`;
    const other = usd ? fmtCount(m.txn_count)+" tx" : fmtUSDC(m.volume_usdc);
    return `<div class="mrow">
      <span class="lab">${m.month}</span>
      <span class="meter"><span class="bar ${m.complete?"":"partial"}" style="width:${wpct}%"></span>
        <span class="val">${usd?fmtUSDC(v):fmtCount(v)+" tx"}</span>${mom}<span class="sub">${other}</span></span>
    </div>`;
  }).join("");
}

/* ———————— 8 VELOCITY ———————— */
export function rVelocity(){
  const days = data.velocity.verified_daily;
  const host = $("#velochart");
  if (!days.length){
    host.innerHTML = `<div class="readout">no verified payments in this window</div>`;
    $("#velostats").innerHTML = "";
    return;
  }
  const W = host.clientWidth || 800, H = 190, padL = 46, padB = 20, padT = 14;
  const max = Math.max(...days.map(d => d[1])) || 1;
  const x = i => padL + (W-padL-8) * (days.length > 1 ? i/(days.length-1) : 0.5);
  const y = v => padT + (H-padT-padB) * (1 - v/max);
  let grid = "";
  // gridlines at quarter-multiples of the max, like the daily tape — derived
  // from the data so the axis stays labeled at any traffic level
  [1,2,3].map(k => max*k/4).forEach(v => {
    grid += `<line class="gl" x1="${padL}" y1="${y(v)}" x2="${W-8}" y2="${y(v)}"/><text x="${padL-6}" y="${y(v)+3}" text-anchor="end">${fmtCount(v)}</text>`; });
  let ticks = "";
  days.forEach((d,i) => { if (d[0].endsWith("-01")){
    const lab = new Date(d[0]+"T00:00:00").toLocaleString("en-US",{month:"short"}).toUpperCase();
    ticks += `<text x="${x(i)+3}" y="${H-6}">${lab}</text>`; }});
  const lines = days.map((d,i) => `<line x1="${x(i)}" y1="${H-padB}" x2="${x(i)}" y2="${y(d[1])}" stroke="var(--line-2)" stroke-width="2"/>`).join("");
  const dots = days.map((d,i) => `<rect x="${x(i)-1.4}" y="${y(d[2])-1.4}" width="2.8" height="2.8" fill="var(--agentic)"/>`).join("");
  const pi = peakIndex(days.map(d => d[1]));
  // Flip the annotation anchor when the label would clip past the right edge.
  // Estimate text width conservatively at 7.5px per character (monospace).
  const annotText = `${days[pi][0]} — ${fmtInt(days[pi][1])}/min`;
  const estAnnotW = annotText.length * 7.5;
  const flipAnnot = (x(pi) + 10 + estAnnotW) > (W - 8);
  const annotAttrs = flipAnnot
    ? `x="${x(pi)-10}" text-anchor="end"`
    : `x="${x(pi)+10}"`;
  host.innerHTML = `<svg class="ch" viewBox="0 0 ${W} ${H}" height="${H}" id="vsvg">
    ${grid}${lines}${dots}
    <circle cx="${x(pi)}" cy="${y(days[pi][1])}" r="4" fill="none" stroke="var(--contam)" stroke-width="1.3"/>
    <text ${annotAttrs} y="${y(days[pi][1])+4}" style="fill:var(--contam);font-weight:700">${annotText}</text>
    ${ticks}
    <line class="ax" x1="${padL}" y1="${H-padB}" x2="${W-8}" y2="${H-padB}"/>
    <rect id="vhover" x="${padL}" y="${padT}" width="${W-padL-8}" height="${H-padT-padB}" fill="transparent"/>
  </svg>`;
  const svg = $("#vsvg");
  $("#vhover").addEventListener("mousemove", e => {
    const r = svg.getBoundingClientRect();
    const sx = (e.clientX - r.left) * (W/r.width);
    const i = Math.max(0, Math.min(days.length-1, Math.round((sx-padL)/((W-padL-8)/(days.length-1)))));
    $("#v-readout").innerHTML = `<span class="d">${days[i][0]}</span> ▸ max ${fmtInt(days[i][1])}/min ▸ p99 ${fmtInt(days[i][2])}/min`;
  });
  $("#vhover").addEventListener("mouseleave", () => { $("#v-readout").textContent = "│ = day's max minute · ▪ = p99 of active minutes"; });
  const vw = data.velocity.windows;
  $("#velostats").innerHTML = ["7d","30d","all"].map(k =>
    `<div class="kv"><span class="k">peak · ${k.toUpperCase()}</span><span class="v">${fmtInt(vw[k].max_per_min)}/min</span></div>`).join("") +
    `<div class="kv"><span class="k">median p99 (active)</span><span class="v">${fmtInt(medianOf(days.map(d => d[2])))}/min</span></div>`;
}

/* ———————— 6.4 GAS COST DAILY ———————— */
// rGasCostDaily renders a small line chart of settlement cost (cents per $1 moved)
// over time inside #gascost-chart. Old artifacts without gas.cost_daily show a
// "not in this artifact" fallback — this section is NOT in REQUIRED_SECTIONS.
export function rGasCostDaily(){
  const days = data.gas.cost_daily;
  const host = $("#gascost-chart");
  if (!host) return;
  if (!days || !days.length){
    host.innerHTML = `<div class="readout">cost-daily series not in this artifact — re-emit after rollup</div>`;
    return;
  }
  const W = host.clientWidth || 800, H = 130, padL = 46, padB = 20, padT = 10;
  const vals = days.map(d => d[1]);
  const vmax = Math.max(...vals) || 1;
  const x = i => padL + (W-padL-8) * (days.length > 1 ? i/(days.length-1) : 0.5);
  const y = v => padT + (H-padT-padB) * (1 - v/vmax);
  // gridlines at quarter-multiples of the max
  let grid = "";
  [1,2,3].map(k => vmax*k/4).forEach(v => {
    grid += `<line class="gl" x1="${padL}" y1="${y(v)}" x2="${W-8}" y2="${y(v)}"/><text x="${padL-6}" y="${y(v)+3}" text-anchor="end">${v.toFixed(1)}</text>`;
  });
  // x-axis month ticks for long series; sparse date ticks for short
  let ticks = "";
  if (days.length > 60){
    days.forEach((d,i) => { if (d[0].endsWith("-01")){
      const lab = new Date(d[0]+"T00:00:00").toLocaleString("en-US",{month:"short"}).toUpperCase();
      ticks += `<text x="${x(i)+3}" y="${H-6}">${lab}</text>`; }});
  } else {
    const step = Math.max(1, Math.round(days.length/5));
    for (let i=0; i<days.length; i+=step){
      const edge = i >= days.length - step;
      const tx = edge ? `x="${x(i)-4}" text-anchor="end"` : `x="${x(i)+4}"`;
      const lab = new Date(days[i][0]+"T00:00:00Z").toLocaleString("en-US",{month:"short",day:"2-digit",timeZone:"UTC"}).toUpperCase();
      ticks += `<text ${tx} y="${H-6}">${lab}</text>`;
    }
  }
  // line path
  let p = "";
  days.forEach((d,i) => { const xx = x(i), yy = y(d[1]); p += i===0 ? `M${xx},${yy}` : `L${xx},${yy}`; });
  // edge day marker (incomplete = d[2] === false)
  const last = days[days.length-1];
  const edgeMarker = last[2] === false
    ? `<circle data-partial="true" cx="${x(days.length-1)}" cy="${y(last[1])}" r="3.5" fill="none" stroke="var(--faint)" stroke-width="1.2" stroke-dasharray="2 2" opacity="0.6"/>`
    : "";
  host.innerHTML = `<svg class="ch" viewBox="0 0 ${W} ${H}" height="${H}" id="gcsvg">
    ${grid}${ticks}
    <path d="${p}" fill="none" stroke="var(--contam)" stroke-width="1.4"/>
    ${edgeMarker}
    <line class="ax" x1="${padL}" y1="${H-padB}" x2="${W-8}" y2="${H-padB}"/>
    <rect id="gchover" x="${padL}" y="${padT}" width="${W-padL-8}" height="${H-padT-padB}" fill="transparent"/>
  </svg>`;
  const svg = $("#gcsvg");
  if (!svg) return;
  const hoverRect = $("#gchover");
  if (!hoverRect) return;
  hoverRect.addEventListener("mousemove", e => {
    const r = svg.getBoundingClientRect();
    const sx = (e.clientX - r.left) * (W/r.width);
    const i = Math.max(0, Math.min(days.length-1, Math.round((sx-padL)/((W-padL-8)/(days.length-1 || 1)))));
    const d = days[i];
    const readout = document.getElementById("gascost-readout");
    if (readout) readout.innerHTML = `<span class="d">${d[0]}</span> ▸ ${(+d[1]).toFixed(2)}¢ per 1 USDC${d[2]===false ? " · partial day" : ""}`;
  });
  hoverRect.addEventListener("mouseleave", () => {
    const readout = document.getElementById("gascost-readout");
    if (readout) readout.textContent = "hover ▸ cents per 1 USDC moved";
  });
}

/* ———————— 9 ACTIVE WALLETS ———————— */
export function rActiveWallets(){
  const ae = data.active_entities;
  const host = $("#aechart");
  if (!ae || !ae.length){
    host.innerHTML = `<div class="readout">active wallet series not in this artifact — re-emit after rollup</div>`;
    return;
  }
  const W = host.clientWidth || 800, H = 190, padL = 62, padB = 20, padT = 14;
  const maxPayers = Math.max(...ae.map(d => d.payer_count)) || 1;
  const maxPayees = Math.max(...ae.map(d => d.payee_count)) || 1;
  const vmax = Math.max(maxPayers, maxPayees);
  const x = i => padL + (W-padL-8) * (ae.length > 1 ? i/(ae.length-1) : 0.5);
  const y = v => padT + (H-padT-padB) * (1 - v/vmax);
  let grid = "";
  [1,2,3].map(k => vmax*k/4).forEach(v => {
    grid += `<line class="gl" x1="${padL}" y1="${y(v)}" x2="${W-8}" y2="${y(v)}"/><text x="${padL-6}" y="${y(v)+3}" text-anchor="end">${fmtCount(v)}</text>`;
  });
  let ticks = "";
  ae.forEach((d,i) => { if (d.day.endsWith("-01")){
    const lab = new Date(d.day+"T00:00:00").toLocaleString("en-US",{month:"short"}).toUpperCase();
    ticks += `<text x="${x(i)+3}" y="${H-6}">${lab}</text>`; }});
  // Payer line (agentic green); payee line (accent, dashed).
  let payerPath = "", payeePath = "";
  ae.forEach((d,i) => {
    const xx = x(i), yp = y(d.payer_count), yq = y(d.payee_count);
    payerPath += i===0 ? `M${xx},${yp}` : `L${xx},${yp}`;
    payeePath += i===0 ? `M${xx},${yq}` : `L${xx},${yq}`;
  });
  // Edge day marker (last point, complete=false).
  const last = ae[ae.length-1];
  const edgeMarker = !last.complete
    ? `<circle data-partial="true" cx="${x(ae.length-1)}" cy="${y(last.payer_count)}" r="3.5" fill="none" stroke="var(--faint)" stroke-width="1.2" stroke-dasharray="2 2" opacity="0.6"/>
       <circle data-partial="true" cx="${x(ae.length-1)}" cy="${y(last.payee_count)}" r="3.5" fill="none" stroke="var(--faint)" stroke-width="1.2" stroke-dasharray="2 2" opacity="0.6"/>`
    : "";
  host.innerHTML = `<svg class="ch" viewBox="0 0 ${W} ${H}" height="${H}" id="aesvg">
    ${grid}${ticks}
    <path d="${payerPath}" fill="none" stroke="var(--agentic)" stroke-width="1.4"/>
    <path d="${payeePath}" fill="none" stroke="var(--accent)" stroke-width="1.4" stroke-dasharray="4 2"/>
    ${edgeMarker}
    <line class="ax" x1="${padL}" y1="${H-padB}" x2="${W-8}" y2="${H-padB}"/>
    <rect id="aehover" x="${padL}" y="${padT}" width="${W-padL-8}" height="${H-padT-padB}" fill="transparent"/>
  </svg>
  <div class="ae-legend">
    <span style="color:var(--agentic)">— payers</span>
    <span style="color:var(--accent);margin-left:12px">-- payees</span>
  </div>`;
  const svg = $("#aesvg");
  if (!svg) return;
  const hoverRect = $("#aehover");
  if (!hoverRect) return;
  hoverRect.addEventListener("mousemove", e => {
    const r = svg.getBoundingClientRect();
    const sx = (e.clientX - r.left) * (W/r.width);
    const i = Math.max(0, Math.min(ae.length-1, Math.round((sx-padL)/((W-padL-8)/(ae.length-1 || 1)))));
    const d = ae[i];
    const readout = document.getElementById("ae-readout");
    if (readout) readout.innerHTML = `<span class="d">${d.day}</span> ▸ payers ${fmtInt(d.payer_count)} ▸ payees ${fmtInt(d.payee_count)}`;
  });
  hoverRect.addEventListener("mouseleave", () => {
    const readout = document.getElementById("ae-readout");
    if (readout) readout.textContent = "payers (solid) · payees (dashed) · per day";
  });
}
