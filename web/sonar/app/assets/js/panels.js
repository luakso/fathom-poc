// Non-chart panel renderers, moved verbatim from the mockup.
import { $ } from "./dom.js";
import { num, fmtInt, fmtMoney, fmtMoneyFull, fmtCount, fmtAmt, pct, BANDDEF, priceRead, claimVerdict } from "./format.js";
import { USD_TOLERANCE } from "./adapter.js";
import { state, data, winLabel } from "./state.js";

/* ———————— 1 OVERVIEW ———————— */
function fmtExcluded(ex) {
  if (!ex || !ex.txn_count) return "";
  const n = ex.txn_count;
  const nStr = n >= 1e6 ? (n/1e6).toFixed(1)+"M" : n >= 1e3 ? (n/1e3).toFixed(0)+"k" : String(n);
  const v = num(ex.volume_usdc);
  const vStr = v >= 1e9 ? "$"+(v/1e9).toFixed(1)+"B" : v >= 1e6 ? "$"+(v/1e6).toFixed(0)+"M" : "$"+(v/1e3).toFixed(0)+"k";
  return `Transfers we can't tie to a known facilitator (${nStr} transfers, ${vStr}, in the same period) are excluded from every number here.`;
}

export function rOverview(){
  const w = data.windows[state.win];
  const t = data.typical[state.win];
  const payees = (((data.concentration.windows[state.win] || {}).payee) || {}).total_entities || 0;
  $("#ov-win").textContent = "· " + winLabel[state.win];
  $("#ov-stats").innerHTML = `
    <div class="bignum c-ag glow">${fmtCount(w.txn_count)}<small>VERIFIED PAYMENTS</small></div>
    <div class="bignum c-ag">${fmtMoney(w.volume_usdc)}<small>VOLUME</small></div>
    <div class="bignum">${fmtAmt(t.median_usdc)}<small>TYPICAL PAYMENT (median)</small><small style="display:block;font-size:.65em;opacity:.65;margin-top:2px">${fmtAmt(t.avg_usdc)} average — pulled up by large payments</small></div>
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
  $("#shp-big").innerHTML = `
    <div class="bignum c-ag glow">${fmtAmt(t.median_usdc)}<small>MEDIAN PAYMENT</small></div>
    <div class="bignum">${fmtAmt(t.avg_usdc)}<small>MEAN — ${isFinite(xMed) ? Math.round(xMed).toLocaleString() : "—"}× THE MEDIAN</small></div>`;
  const b = data.windows[state.win].by_band;
  const tx = state.bMetric === "tx";
  const get = r => tx ? r.txn_count : num(r.volume_usdc);
  const max = Math.max(...BANDDEF.map(([k]) => get(b[k]))) || 1;
  $("#bands").innerHTML = BANDDEF.map(([k,def]) => {
    const r = b[k], v = get(r), wp = Math.max(1, 66*v/max);
    return `<div class="mrow">
      <span class="lab">${k}<small>${def}</small></span>
      <span class="meter"><span class="bar ${k==="whale"||k==="mid"?"dim":""}" style="width:${wp}%"></span>
      <span class="val">${tx?fmtCount(v)+" tx":fmtMoney(v)}</span><span class="sub">${tx?fmtMoney(r.volume_usdc):fmtCount(r.txn_count)+" tx"}</span></span></div>`;
  }).join("");
}

/* ———————— 6 PRICE POINTS ———————— */
export function rPrice(){
  const pts = data.price_points[state.win];
  $("#pp-win").textContent = "· " + winLabel[state.win];
  if (!pts.length){
    $("#pptable").innerHTML = `<tbody><tr><td style="color:var(--faint);padding:14px 0">no verified payments in this window</td></tr></tbody>`;
    return;
  }
  const maxS = Math.max(...pts.map(p => num(p.txn_share_pct))) || 1;
  const TAG = { menu:`<span class="tag menu">MENU</span>`, market:`<span class="tag market">MARKET</span>`, mixed:`<span class="tag mixed">—</span>` };
  $("#pptable").innerHTML = `
    <thead><tr><th>amount</th><th style="text-align:left">share of verified tx</th><th>tx</th><th>payees</th><th>read</th></tr></thead>
    <tbody>${pts.map(p => {
      const s = num(p.txn_share_pct), w = Math.max(1.5, 100*s/maxS);
      const tag = TAG[priceRead(p)];
      return `<tr><td style="font-weight:700">${fmtAmt(p.amount_usdc)}</td>
        <td style="text-align:left;min-width:140px"><span style="display:inline-block;vertical-align:middle;height:9px;width:${w}px;max-width:60%;background:var(--agentic)"></span> <span style="color:var(--dim)">${s.toFixed(1)}%</span></td>
        <td>${fmtInt(p.txn_count)}</td>
        <td${p.payee_count<10?' style="color:var(--contam);font-weight:700"':""}>${fmtInt(p.payee_count)}</td>
        <td>${tag}</td></tr>`;
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
    <div class="kv"><span class="k">value moved</span><span class="v">${fmtMoneyFull(vol)}</span></div>
    <div class="kv"><span class="k">cost per $1 settled</span><span class="v">${kn.gas_cents_per_dollar === null ? "—" : num(kn.gas_cents_per_dollar).toFixed(2)+"¢"}</span></div>
    <div class="kv"><span class="k">breakeven payments</span><span class="v c-cm">${fmtInt(kn.breakeven_txn_count)} <small>of ${fmtInt(kn.txn_count)}</small></span></div>`;
  $("#gasbands").innerHTML = `
    <thead><tr><th>band</th><th>cost¢/$1</th><th>breakeven</th></tr></thead>
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
    const ratio = num(c.claimed_value)/num(c.measured_value);
    const verdict = claimVerdict(ratio);
    const tagClass = ratio >= 1.5 ? "over" : ratio < 0.9 ? "low" : "ok";
    const tag = `<span class="tag ${tagClass}">claim ${verdict}</span>`;
    const isUsd = (c.measured_unit || "").toUpperCase() === "USDC";
    const fmt = isUsd ? fmtMoney : fmtInt;
    return `<div class="claimrow">
      <div class="q">"${c.claim_text}"</div>
      <div class="src">${/^https?:\/\//i.test(c.source_url) ? `<a href="${c.source_url.replace(/"/g,"%22")}" target="_blank" rel="noopener" style="color:inherit">${c.source}</a>` : c.source} · ${c.claim_date} · measured as ${c.measured_metric}</div>
      <div class="nums"><span class="a">claimed ${fmt(c.claimed_value)}</span><span>▸</span><span class="m">measured ${fmt(c.measured_value)}</span>${tag}</div>
    </div>`;
  }).join("");
}

/* ———————— LOG ———————— */
export function rShell(){
  const w = data.windows.all;
  $("#shell").innerHTML = `
    <div><span class="ps">$</span> <span class="cmd">jq '.scope, .methodology_version, .data_through_day' dist/economy.json</span></div>
    <div class="out">x402-attributed · v${data.meta.methodology_version} · ${data.meta.data_through_day}</div>
    <div class="out">${fmtInt(w.txn_count)} verified payments · ${fmtMoney(w.volume_usdc)} <span class="ok">✓</span></div>
    <div class="out">every shown payment is settled by a known x402 facilitator.<span class="cursor" style="margin-left:6px"></span></div>`;
}
