// Non-chart panel renderers, moved verbatim from the mockup.
import { $ } from "./dom.js";
import { num, fmtInt, fmtMoneyFull, fmtUSDC, fmtUSDCFull, fmtUSDCAmt, fmtCount, pct, BANDDEF, priceRead, claimVerdict, escHtml } from "./format.js";
import { USD_TOLERANCE } from "./adapter.js";
import { state, data, winLabel, issues, facData } from "./state.js";

/* ———————— 1 OVERVIEW ———————— */
function fmtExcluded(ex) {
  if (!ex || !ex.txn_count) return "";
  const n = ex.txn_count;
  const nStr = n >= 1e6 ? (n/1e6).toFixed(1)+"M" : n >= 1e3 ? (n/1e3).toFixed(0)+"k" : String(n);
  const v = num(ex.volume_usdc);
  const vStr = v >= 1e9 ? (v/1e9).toFixed(1)+"B USDC" : v >= 1e6 ? (v/1e6).toFixed(0)+"M USDC" : (v/1e3).toFixed(0)+"k USDC";
  return `Transfers we can't tie to a known facilitator (${nStr} transfers, ${vStr}, in the same period) are excluded from every number here.`;
}

export function rOverview(){
  const w = data.windows[state.win];
  const t = data.typical[state.win];
  const payees = (((data.concentration.windows[state.win] || {}).payee) || {}).total_entities || 0;
  // 6.2: largest payment — omit when absent (old-artifact tolerance).
  const largestStat = t.largest_payment_usdc != null
    ? `<div class="bignum">${fmtUSDC(t.largest_payment_usdc)}<small>LARGEST PAYMENT</small></div>`
    : "";
  $("#ov-win").textContent = "· " + winLabel[state.win];
  $("#ov-stats").innerHTML = `
    <div class="bignum c-ag glow">${fmtCount(w.txn_count)}<small>VERIFIED PAYMENTS</small></div>
    <div class="bignum c-ag">${fmtUSDC(w.volume_usdc)}<small>VOLUME</small></div>
    <div class="bignum">${fmtUSDCAmt(t.median_usdc)}<small>TYPICAL PAYMENT (median)</small><small style="display:block;font-size:.65em;opacity:.65;margin-top:2px">${fmtUSDCAmt(t.avg_usdc)} average — pulled up by large payments</small></div>
    ${largestStat}
    <div class="bignum">${fmtInt(payees)}<small>ACTIVE PAYEES</small></div>`;
  // amount-band distribution bar (share of verified volume by band)
  const bands = BANDDEF.map(([k]) => [k, num(w.by_band[k].volume_usdc)]);
  const totalV = bands.reduce((s,[,v]) => s+v, 0) || 1;
  $("#ov-bands").innerHTML = bands.map(([k,v]) => {
    const p = 100*v/totalV;
    return `<span class="seg" style="width:${p}%" title="${k} ${p.toFixed(1)}% of volume"><b>${k}</b></span>`;
  }).join("");
  const defn = "A verified payment is a USDC payment settled on Base by a known x402 facilitator.";
  const excl = fmtExcluded(data.excluded);
  const denomParts = [defn, excl, winLabel[state.win] + " · windows anchored to data-through day"].filter(Boolean);
  $("#ov-denom").textContent = denomParts.join(" · ");
}

/* ———————— 5 SHAPE ———————— */
export function rShape(){
  const t = data.typical[state.win];
  const xMed = num(t.avg_usdc)/num(t.median_usdc);
  $("#shp-win").textContent = "· " + winLabel[state.win];
  // 6.3 percentile strip: shown only when the artifact carries p10/p90/p99.
  // Old-artifact tolerance: missing or null fields → strip omitted, not a crash.
  const pctStrip = t.p10_usdc != null && t.p90_usdc != null && t.p99_usdc != null
    ? `<div class="pct-strip" style="margin-top:6px;font-size:.8em;color:var(--dim)">p10 ${fmtUSDCAmt(t.p10_usdc)} · median ${fmtUSDCAmt(t.median_usdc)} · p90 ${fmtUSDCAmt(t.p90_usdc)} · p99 ${fmtUSDCAmt(t.p99_usdc)}</div>`
    : "";
  $("#shp-big").innerHTML = `
    <div class="bignum c-ag glow">${fmtUSDCAmt(t.median_usdc)}<small>MEDIAN PAYMENT</small></div>
    <div class="bignum">${fmtUSDCAmt(t.avg_usdc)}<small>MEAN — ${isFinite(xMed) ? Math.round(xMed).toLocaleString() : "—"}× THE MEDIAN</small></div>
    ${pctStrip}`;
  const b = data.windows[state.win].by_band;
  const tx = state.bMetric === "tx";
  const get = r => tx ? r.txn_count : num(r.volume_usdc);
  const max = Math.max(...BANDDEF.map(([k]) => get(b[k]))) || 1;
  const stackBar = (pairs, lbl) => {
    const tot = pairs.reduce((s,[,v]) => s+v, 0) || 1;
    return `<div class="bbrow"><span class="bblbl">${lbl}</span><div class="bandbar">${
      pairs.map(([k,v]) => `<span class="seg" style="width:${(100*v/tot).toFixed(2)}%" title="${k} ${(100*v/tot).toFixed(1)}%"><b>${k}</b></span>`).join("")
    }</div></div>`;
  };
  const txBands  = BANDDEF.map(([k]) => [k, b[k].txn_count]);
  const usdBands = BANDDEF.map(([k]) => [k, num(b[k].volume_usdc)]);
  $("#bands").innerHTML = stackBar(txBands, "share of payments") + stackBar(usdBands, "share of dollars")
    + BANDDEF.map(([k,def]) => {
      const r = b[k], v = get(r), wp = Math.max(1, 66*v/max);
      return `<div class="mrow">
        <span class="lab">${k}<small>${def}</small></span>
        <span class="meter"><span class="bar ${k==="whale"||k==="mid"?"dim":""}" style="width:${wp}%"></span>
        <span class="val">${tx?fmtCount(v)+" tx":fmtUSDC(v)}</span><span class="sub">${tx?fmtUSDC(r.volume_usdc):fmtCount(r.txn_count)+" tx"}</span></span></div>`;
    }).join("");
}

/* ———————— 6 PRICE POINTS ———————— */

// sparklineSVG: tiny inline SVG polyline of payee counts over time.
// Uses the velocity-chart SVG idiom (reused, not invented). Width 60×16.
function sparklineSVG(series){
  if (!series || series.length < 2) return '<span style="color:var(--faint);font-size:.75em">—</span>';
  const counts = series.map(d => d.payee_count);
  const mx = Math.max(...counts) || 1;
  const mn = Math.min(...counts);
  const range = mx - mn || 1;
  const W = 60, H = 16;
  const pts = counts.map((v, i) => {
    const x = (i * (W - 2) / (counts.length - 1) + 1).toFixed(1);
    const y = (H - 1 - ((v - mn) / range) * (H - 2)).toFixed(1);
    return `${x},${y}`;
  }).join(" ");
  return `<svg width="${W}" height="${H}" viewBox="0 0 ${W} ${H}" style="display:inline-block;vertical-align:middle"><polyline points="${pts}" fill="none" stroke="var(--agentic)" stroke-width="1.5"/></svg>`;
}

export function rPrice(){
  const pts = data.price_points[state.win];
  $("#pp-win").textContent = "· " + winLabel[state.win];
  if (!pts.length){
    $("#pptable").innerHTML = `<tbody><tr><td style="color:var(--faint);padding:14px 0">no verified payments in this window</td></tr></tbody>`;
    return;
  }
  // 6.7: look up price_breadth series for sparklines; tolerate absence (older artifacts).
  const breadth = data.price_breadth || null;
  const hasBreadth = !!breadth;
  const maxS = Math.max(...pts.map(p => num(p.txn_share_pct))) || 1;
  const TAG = { menu:`<span class="tag menu">MENU</span>`, market:`<span class="tag market">MARKET</span>`, mixed:`<span class="tag mixed">—</span>` };
  const breadthHead = hasBreadth ? "<th>payees trend</th>" : "";
  $("#pptable").innerHTML = `
    <thead><tr><th>amount</th><th style="text-align:left">share of verified tx</th><th>tx</th><th>payees</th>${breadthHead}<th>read</th></tr></thead>
    <tbody>${pts.map(p => {
      const s = num(p.txn_share_pct), w = Math.max(1.5, 100*s/maxS);
      const tag = TAG[priceRead(p)];
      let sparkCell = "";
      if (hasBreadth){
        const bs = breadth.find(b => b.amount_usdc === p.amount_usdc);
        sparkCell = `<td>${sparklineSVG(bs ? bs.series : null)}</td>`;
      }
      return `<tr><td style="font-weight:700">${fmtUSDCAmt(p.amount_usdc)}</td>
        <td style="text-align:left;min-width:140px"><span style="display:inline-block;vertical-align:middle;height:9px;width:${w}px;max-width:60%;background:var(--agentic)"></span> <span style="color:var(--dim)">${s.toFixed(1)}%</span></td>
        <td>${fmtInt(p.txn_count)}</td>
        <td${p.payee_count<10?' style="color:var(--contam);font-weight:700"':""}>${fmtInt(p.payee_count)}</td>
        ${sparkCell}<td>${tag}</td></tr>`;
    }).join("")}</tbody>`;
}

/* ———————— 7 GAS ———————— */
export function rGas(){
  const g = data.gas.windows[state.win], kn = g;
  const p = 100*kn.breakeven_txn_count/kn.txn_count;
  $("#gas-win").textContent = "· " + winLabel[state.win];
  $("#gas-pct").textContent = isFinite(p) ? p.toFixed(1) + "%" : "—";
  const cells = 40, f = isFinite(p) ? Math.min(cells, Math.round(cells*p/100)) : 0;
  $("#gas-meter").innerHTML = `<span class="f">${"▓".repeat(f)}</span><span class="e">${"░".repeat(cells-f)}</span>`;
  const vol = data.windows[state.win].volume_usdc;
  $("#gas-kv").innerHTML = `
    <div class="kv"><span class="k">cost L1+L2</span><span class="v">${num(kn.gas_eth).toFixed(3)} ETH <small>≈ ${fmtMoneyFull(kn.gas_usd)}</small></span></div>
    <div class="kv"><span class="k">L1 / L2 (ETH)</span><span class="v">${num(kn.gas_eth_l1).toFixed(3)} / ${num(kn.gas_eth_l2).toFixed(3)}</span></div>
    <div class="kv"><span class="k">value moved</span><span class="v">${fmtUSDCFull(vol)}</span></div>
    <div class="kv"><span class="k">cost per 1 USDC settled</span><span class="v">${kn.gas_cents_per_dollar === null ? "—" : num(kn.gas_cents_per_dollar).toFixed(2)+"¢"}</span></div>
    <div class="kv"><span class="k">breakeven payments</span><span class="v c-cm">${fmtInt(kn.breakeven_txn_count)} <small>of ${fmtInt(kn.txn_count)}</small></span></div>`;
  $("#gasbands").innerHTML = `
    <thead><tr><th>band</th><th>cost¢/USDC</th><th>breakeven</th></tr></thead>
    <tbody>${BANDDEF.map(([k,def]) => {
      const r = g.by_band[k]; if (!r) return "";
      const c = r.gas_cents_per_dollar === null ? "—" : num(r.gas_cents_per_dollar).toFixed(2)+"¢";
      const hot = r.gas_cents_per_dollar !== null && num(r.gas_cents_per_dollar) > 100;
      return `<tr><td style="font-weight:700">${k} <span style="color:var(--faint)">${def}</span></td>
        <td${hot?' style="color:var(--contam);font-weight:700"':""}>${c}${hot?" ⚠":""}</td>
        <td>${pct(r.breakeven_txn_count, r.txn_count)}</td></tr>`;
    }).join("")}</tbody>`;
}

/* ———————— 9 CLAIMS ———————— */
export function rClaims(){
  if (!data.claims.length){
    $("#claims").innerHTML = `<div style="color:var(--faint);padding:14px 0">no claims curated — data/claims.json is empty for this publish</div>`;
    return;
  }
  $("#claims").innerHTML = data.claims.map(c => {
    const measured = num(c.measured_value);
    const ratio = measured !== 0 ? num(c.claimed_value) / measured : null;
    const verdict = claimVerdict(ratio);
    const tagClass = ratio === null ? "na" : ratio >= 1.5 ? "over" : ratio < 0.9 ? "low" : "ok";
    const tag = `<span class="tag ${tagClass}">claim ${verdict}</span>`;
    const isUsd = (c.measured_unit || "").toUpperCase() === "USDC";
    const fmt = isUsd ? fmtUSDC : fmtInt;
    return `<div class="claimrow">
      <div class="q">"${escHtml(c.claim_text)}"</div>
      <div class="src">${/^https?:\/\//i.test(c.source_url) ? `<a href="${escHtml(c.source_url)}" target="_blank" rel="noopener" style="color:inherit">${escHtml(c.source)}</a>` : escHtml(c.source)} · ${escHtml(c.claim_date)} · measured as ${escHtml(c.measured_metric)}</div>
      <div class="nums"><span class="a">claimed ${fmt(c.claimed_value)}</span><span>▸</span><span class="m">measured ${fmt(c.measured_value)}</span>${tag}</div>
    </div>`;
  }).join("");
}

/* ———————— 9b PAYER COHORTS (6.5) ———————— */
// Renders two-segment volume bars (new vs returning) for the 7d and 30d windows.
// Writes into #ae-cohort; renders nothing when payer_cohorts is absent (old-artifact tolerance).
export function rPayerCohorts(){
  const el = $("#ae-cohort");
  if (!el) return;
  const cohorts = data.payer_cohorts;
  if (!cohorts) { el.innerHTML = ""; return; }
  const rows = ["7d", "30d"].map(wn => {
    const c = cohorts[wn];
    if (!c) return "";
    const nv = num(c.new_payer_volume_usdc);
    const rv = num(c.returning_payer_volume_usdc);
    const tot = nv + rv || 1;
    const np = (100 * nv / tot).toFixed(1);
    const rp = (100 * rv / tot).toFixed(1);
    return `<div class="cohort-row">
      <span class="cohort-lbl">${wn}</span>
      <div class="cohort-bar">
        <span class="seg" style="width:${np}%;background:var(--agentic)" title="new payers ${np}% of volume"><b>new</b></span>
        <span class="seg" style="width:${rp}%;background:var(--dim)" title="returning payers ${rp}% of volume"><b>returning</b></span>
      </div>
      <span class="cohort-pct c-ag">${np}% new</span>
    </div>`;
  }).join("");
  el.innerHTML = rows;
}

/* ———————— 10 FACILITATORS ———————— */
// Shortens an Ethereum address to 0xabcd…1234 for table display.
function shortAddr(addr){ return String(addr).length > 10 ? String(addr).slice(0,6) + "…" + String(addr).slice(-4) : addr; }

// rFacilitators renders the panel from the secondary facilitators.json fetch.
// If facData is null (fetch failed) or windows are absent (old artifact), it
// renders an explicit absent-state — the rest of the page is unaffected.
export function rFacilitators(){
  const el = $("#fac-body");
  if (!el) return;
  if (!facData || !facData.rows || !facData.rows.length){
    el.innerHTML = `<div style="color:var(--faint);padding:14px 0">facilitator data unavailable — check that dist/facilitators.json was emitted</div>`;
    return;
  }
  const hasWindows = !!(facData.rows[0].windows && facData.rows[0].windows["7d"] && facData.rows[0].windows["30d"]);
  if (!hasWindows){
    el.innerHTML = `<div style="color:var(--faint);padding:14px 0">facilitator window data absent — re-emit after rolling up</div>`;
    return;
  }
  const show = facData.rows.slice(0, 8);
  const more = facData.rows.length - 8;
  const momentum = r => {
    const v7  = num(r.windows["7d"].volume_usdc);
    const v30 = num(r.windows["30d"].volume_usdc);
    return v30 > 0 ? (100 * v7 / v30).toFixed(0) + "%" : "—";
  };
  el.innerHTML = `<table class="t" style="width:100%">
    <thead><tr><th>facilitator</th><th>all tx</th><th>all volume</th><th>7d volume</th><th>momentum</th></tr></thead>
    <tbody>${show.map(r => `<tr>
      <td style="font-family:monospace;font-size:.85em">${escHtml(shortAddr(r.facilitator))}</td>
      <td>${fmtCount(r.txn_count)}</td>
      <td>${fmtUSDC(r.volume_usdc)}</td>
      <td>${fmtUSDC(r.windows["7d"].volume_usdc)}</td>
      <td style="color:var(--dim)">${momentum(r)}</td>
    </tr>`).join("")}
    ${more > 0 ? `<tr><td colspan="5" style="color:var(--faint);padding-top:4px">…${fmtInt(more)} more facilitators</td></tr>` : ""}
    </tbody>
  </table>`;
}

/* ———————— LOG ———————— */
export function rShell(){
  const w = data.windows.all;
  const allPass = issues.every(c => c.pass);
  const glyph = allPass ? `<span class="ok">✓</span>` : `<span class="fail">✗</span>`;
  const lines = issues.map(c => {
    const cls = c.pass ? "ok" : "fail";
    return `<div class="out"><span class="${cls}">${c.pass ? "✓" : "✗"}</span> ${c.detail}</div>`;
  }).join("");
  $("#shell").innerHTML = `
    <div><span class="ps">$</span> <span class="cmd">jq '.scope, .methodology_version, .data_through_day' dist/economy.json</span></div>
    <div class="out">${data.meta.scope || "verified-x402"} · v${data.meta.methodology_version} · ${data.meta.data_through_day}</div>
    ${lines}
    <div class="out">${fmtInt(w.txn_count)} verified payments · ${fmtUSDC(w.volume_usdc)} ${glyph}<span class="cursor" style="margin-left:6px"></span></div>`;
}
