// Non-chart panel renderers, moved verbatim from the mockup.
import { $ } from "./dom.js";
import { num, fmtInt, fmtMoney, fmtMoneyFull, fmtCount, fmtAmt, pct, BANDDEF, ATTRS, priceRead, claimVerdict } from "./format.js";
import { USD_TOLERANCE } from "./adapter.js";
import { state, data, winLabel } from "./state.js";

/* ———————— 1 OVERVIEW ———————— */
export function rOverview(){
  const w = data.windows[state.win], a = w.by_attribution;
  $("#ov-win").textContent = "· " + winLabel[state.win];
  $("#ov-stats").innerHTML = `
    <div class="bignum">${fmtCount(w.txn_count)}<small>PAYMENTS OBSERVED</small></div>
    <div class="bignum">${fmtMoney(w.volume_usdc)}<small>VOLUME OBSERVED</small></div>
    <div class="bignum c-ag glow">${fmtMoney(a.agentic.volume_usdc)}<small>AGENTIC USD</small></div>
    <div class="bignum c-ag">${pct(a.agentic.volume_usdc, w.volume_usdc)}<small>AGENTIC SHARE OF $</small></div>`;
  const mk = (el, get, tot) => {
    el.innerHTML = ATTRS.map(([k]) => {
      const p = 100*num(get(a[k]))/num(tot);
      const cls = k==="agentic"?"ag":k==="contested"?"ct":"cm";
      return `<i class="${cls}" style="width:${p}%" title="${k} ${p.toFixed(2)}%"></i>`;
    }).join("");
  };
  mk($("#ov-usd"), x=>x.volume_usdc, w.volume_usdc);
  mk($("#ov-tx"),  x=>x.txn_count,  w.txn_count);
  $("#ov-usd-r").innerHTML = `<span class="c-ag">ag ${pct(a.agentic.volume_usdc,w.volume_usdc)}</span> · <span class="c-ct">ct ${pct(a.contested.volume_usdc,w.volume_usdc)}</span> · <span class="c-cm">cm ${pct(a.contamination.volume_usdc,w.volume_usdc)}</span>`;
  $("#ov-tx-r").innerHTML  = `<span class="c-ag">ag ${pct(a.agentic.txn_count,w.txn_count)}</span> · <span class="c-ct">ct ${pct(a.contested.txn_count,w.txn_count)}</span> · <span class="c-cm">cm ${pct(a.contamination.txn_count,w.txn_count)}</span>`;
  $("#ov-inv").innerHTML = `<b>the inversion:</b> agentic = <span class="c-ag">${pct(a.agentic.txn_count,w.txn_count)}</span> of payments but <span class="c-ag">${pct(a.agentic.volume_usdc,w.volume_usdc)}</span> of dollars — contamination is the mirror image.`;
  $("#ov-denom").textContent = "all observed EIP-3009 USDC on Base · " + winLabel[state.win] + " · windows anchored to data-through day";
}

/* ———————— 4 SPLIT ———————— */
export function rSplit(){
  const w = data.windows[state.win], a = w.by_attribution;
  $("#sp-win").textContent = "· " + winLabel[state.win];
  $("#splittable").innerHTML = `
    <thead><tr><th>class</th><th>tx</th><th>tx%</th><th>usd</th><th>usd%</th></tr></thead>
    <tbody>${ATTRS.map(([k,cls]) => `<tr>
      <td class="${cls}" style="font-weight:700">${k}</td>
      <td>${fmtInt(a[k].txn_count)}</td><td>${pct(a[k].txn_count,w.txn_count)}</td>
      <td>${fmtMoney(a[k].volume_usdc)}</td><td>${pct(a[k].volume_usdc,w.volume_usdc)}</td>
    </tr>`).join("")}
    <tr><td style="color:var(--dim);font-weight:700">total</td>
      <td style="font-weight:700">${fmtInt(w.txn_count)}</td><td>100%</td>
      <td style="font-weight:700">${fmtMoney(w.volume_usdc)}</td><td>100%</td></tr></tbody>`;
  $("#sp-note").innerHTML = `agentic mean ${fmtAmt(data.typical[state.win].agentic.avg_usdc)} vs contamination mean ${fmtAmt(data.typical[state.win].contamination.avg_usdc)} — <b>different species sharing one event signature.</b>`;
  $("#sp-denom").textContent = `allowlist methodology v${data.meta.methodology_version} · full address list in facilitators.json · splits sum to totals exactly`;
}

/* ———————— 5 SHAPE ———————— */
export function rShape(){
  const t = data.typical[state.win];
  const xMed = num(t.agentic.avg_usdc)/num(t.agentic.median_usdc);
  $("#shp-win").textContent = "· " + winLabel[state.win];
  $("#shp-big").innerHTML = `
    <div class="bignum c-ag glow">${fmtAmt(t.agentic.median_usdc)}<small>MEDIAN AGENTIC PAYMENT</small></div>
    <div class="bignum">${fmtAmt(t.agentic.avg_usdc)}<small>MEAN — ${isFinite(xMed) ? Math.round(xMed).toLocaleString() : "—"}× THE MEDIAN</small></div>`;
  $("#typtable").innerHTML = `
    <thead><tr><th>class</th><th>median</th><th>mean</th><th>tx</th></tr></thead>
    <tbody>${[["agentic","c-ag"],["contested","c-ct"],["contamination","c-cm"],["all",""]].map(([k,cls]) => `<tr>
      <td class="${cls}" style="font-weight:700">${k}</td>
      <td${k==="agentic"?' style="color:var(--agentic);font-weight:700"':""}>${fmtAmt(t[k].median_usdc)}</td>
      <td>${fmtAmt(t[k].avg_usdc)}</td><td>${fmtInt(t[k].txn_count)}</td></tr>`).join("")}</tbody>`;
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
    $("#pptable").innerHTML = `<tbody><tr><td style="color:var(--faint);padding:14px 0">no agentic payments in this window</td></tr></tbody>`;
    return;
  }
  const maxS = Math.max(...pts.map(p => num(p.txn_share_pct))) || 1;
  const TAG = { menu:`<span class="tag menu">MENU</span>`, market:`<span class="tag market">MARKET</span>`, mixed:`<span class="tag mixed">—</span>` };
  $("#pptable").innerHTML = `
    <thead><tr><th>amount</th><th style="text-align:left">share of agentic tx</th><th>tx</th><th>payees</th><th>read</th></tr></thead>
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
  const g = data.gas.windows[state.win], ag = g.by_attribution.agentic;
  const p = 100*ag.breakeven_txn_count/ag.txn_count;
  $("#gas-win").textContent = "· " + winLabel[state.win];
  $("#gas-pct").textContent = isFinite(p) ? p.toFixed(1) + "%" : "—";
  const cells = 40, f = isFinite(p) ? Math.min(cells, Math.round(cells*p/100)) : 0;
  $("#gas-meter").innerHTML = `<span class="f">${"▓".repeat(f)}</span><span class="e">${"░".repeat(cells-f)}</span>`;
  const vol = data.windows[state.win].by_attribution.agentic.volume_usdc;
  $("#gas-kv").innerHTML = `
    <div class="kv"><span class="k">gas burned (agentic)</span><span class="v">${num(ag.gas_eth).toFixed(3)} ETH <small>≈ ${fmtMoneyFull(ag.gas_usd)}</small></span></div>
    <div class="kv"><span class="k">value moved (agentic)</span><span class="v">${fmtMoneyFull(vol)}</span></div>
    <div class="kv"><span class="k">gas per $1 settled</span><span class="v">${ag.gas_cents_per_dollar === null ? "—" : num(ag.gas_cents_per_dollar).toFixed(2)+"¢"}</span></div>
    <div class="kv"><span class="k">breakeven payments</span><span class="v c-cm">${fmtInt(ag.breakeven_txn_count)} <small>of ${fmtInt(ag.txn_count)}</small></span></div>`;
  $("#gasbands").innerHTML = `
    <thead><tr><th>band</th><th>gas¢/$1</th><th>breakeven</th></tr></thead>
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
    const tag = `<span class="tag ${verdict === "overstated" ? "over" : "ok"}">×${ratio.toFixed(1)} ${verdict.toUpperCase()}</span>`;
    return `<div class="claimrow">
      <div class="q">“${c.claim_text}”</div>
      <div class="src">${/^https?:\/\//i.test(c.source_url) ? `<a href="${c.source_url.replace(/"/g,"%22")}" target="_blank" rel="noopener" style="color:inherit">${c.source}</a>` : c.source} · ${c.claim_date} · measured as ${c.measured_metric}</div>
      <div class="nums"><span class="a">claimed ${fmtInt(c.claimed_value)}</span><span>▸</span><span class="m">measured ${fmtInt(c.measured_value)}</span>${tag}</div>
    </div>`;
  }).join("");
}

/* ———————— LOG ———————— */
export function rShell(){
  const w = data.windows.all, a = w.by_attribution;
  const sumN = a.agentic.txn_count + a.contested.txn_count + a.contamination.txn_count;
  const sum$ = num(a.agentic.volume_usdc) + num(a.contested.volume_usdc) + num(a.contamination.volume_usdc);
  const okN = sumN === w.txn_count, ok$ = Math.abs(sum$ - num(w.volume_usdc)) <= USD_TOLERANCE;
  $("#shell").innerHTML = `
    <div><span class="ps">$</span> <span class="cmd">fathom verify --conservation --window all</span></div>
    <div class="out">tx&nbsp;: ${fmtInt(a.agentic.txn_count)} + ${fmtInt(a.contested.txn_count)} + ${fmtInt(a.contamination.txn_count)} = ${fmtInt(sumN)} <span class="ok">${okN?"✓":"✗"}</span></div>
    <div class="out">usd: ${fmtMoney(a.agentic.volume_usdc)} + ${fmtMoney(a.contested.volume_usdc)} + ${fmtMoney(a.contamination.volume_usdc)} = ${fmtMoneyFull(sum$)} <span class="ok">${ok$?"✓":"✗"}</span></div>
    <div><span class="ps">$</span> <span class="cmd">jq '.methodology_version, .generated_at, .data_through_day' dist/economy.json</span></div>
    <div class="out">${data.meta.methodology_version} · ${data.meta.generated_at} · ${data.meta.data_through_day}</div>
    <div><span class="ps">$</span> <span class="cmd">fathom catalog --deferred</span></div>
    <div class="out"><span class="warn">E9</span> concentration → entity_rank_v1 (payees/payers round) · <span class="warn">E10</span> december anomaly → re-backfill</div>
    <div class="out">no row dropped — only labeled.<span class="cursor" style="margin-left:6px"></span></div>`;
}
