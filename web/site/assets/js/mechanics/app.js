// web/site/assets/js/mechanics/app.js
// Boots /mechanics: loads mechanics.json (+ economy cross-check), renders unit
// economics, fee intent, batch mechanics, wrapper mix, rails physics, QA canaries,
// verify log. Mirrors reliability/app.js.
import { $, $$ } from "../dom.js";
import { fmtCount, fmtInt, num } from "../format.js";
import { loadMechanics, selectorLabel, txTypeOther } from "../lib/mechanics-adapter.js";
import { createTray } from "../lib/report-tray.js";

const WIN_LABEL = { "7d": "trailing 7d", "30d": "trailing 30d", "all": "all data" };
const state = { win: "all" };
let view = null, issues = [], tray = null;

const GATE_KEY = "fathom.smallScreenOk";
function maybeGate() {
  if (window.innerWidth >= 980 || localStorage.getItem(GATE_KEY) === "1") return;
  const a = view.windows.all;
  $("#gate-nums").innerHTML = `
    <div>${fmtCount(a.settlement_count)}<small>SETTLEMENTS</small></div>
    <div>${pctOf(a.cost.breakeven_txn_count, a.settlement_count)}<small>COST &gt; VALUE</small></div>
    <div>${a.cost.gas_cents_per_dollar == null ? "—" : a.cost.gas_cents_per_dollar}<small>¢ / $</small></div>`;
  $("#gate").classList.add("open");
  $("#gate-continue").addEventListener("click", () => { localStorage.setItem(GATE_KEY, "1"); $("#gate").classList.remove("open"); });
}
function fatal(err) { $("#fatal-msg").textContent = String(err && err.message || err); $("#fatal").classList.add("open"); }
$("#fatal-retry").addEventListener("click", () => location.reload());

const win = () => view.windows[state.win];
function pctOf(n, d) { return d ? (100 * n / d).toFixed(1) + "%" : "0.0%"; }
function gwei(weiStr) { return weiStr == null ? "—" : (num(weiStr) / 1e9).toFixed(4) + " gwei"; }
function secs(v) { return v == null ? "—" : (v < 1 ? v.toFixed(2) + "s" : Math.round(v) + "s"); }
function ratio(v) { return v == null ? "—" : v.toFixed(3); }

function rEconomics() {
  const a = win(), c = a.cost;
  $("#ue-win").textContent = "· " + WIN_LABEL[state.win];
  $("#ue-stats").innerHTML = `
    <div class="bignum c-cm glow">${pctOf(c.breakeven_txn_count, a.settlement_count)}<small>COST &gt; VALUE MOVED</small></div>
    <div class="bignum">${c.gas_cents_per_dollar == null ? "—" : c.gas_cents_per_dollar}<small>¢ COST PER $ SETTLED</small></div>
    <div class="bignum">${num(c.gas_eth).toFixed(2)}<small>ETH BURNED</small></div>
    <div class="bignum">${fmtCount(c.breakeven_txn_count)}<small>UNDERWATER PAYMENTS</small></div>`;
  $("#ue-denom").textContent = `true cost = L2 exec ${num(c.gas_eth_l2).toFixed(2)} + L1 data ${num(c.gas_eth_l1).toFixed(2)} ETH · $${c.gas_usd} · ${WIN_LABEL[state.win]}`;
}

function rFee() {
  const a = win(), t = a.fee.tx_type, other = txTypeOther(a), n = a.settlement_count || 1;
  $("#fee-win").textContent = "· " + WIN_LABEL[state.win];
  const bar = (label, v) => `<div class="kv"><span class="k">${label}</span><span class="v">${fmtCount(v)} · ${pctOf(v, n)}</span></div>`;
  $("#fee-types").innerHTML = bar("type-2 (EIP-1559)", t["2"] || 0) + bar("type-0 (legacy)", t["0"] || 0) + bar("type-1", t["1"] || 0) + bar("other (OP-stack)", other);
  $("#fee-pcts").innerHTML = `
    <div class="bignum">${gwei(a.fee.max_fee.p50)}<small>MAX_FEE P50</small></div>
    <div class="bignum">${gwei(a.fee.max_fee.p99)}<small>MAX_FEE P99</small></div>
    <div class="bignum">${gwei(a.fee.max_priority.p50)}<small>TIP P50</small></div>`;
}

function rBatch() {
  const a = win(), b = a.batch, hist = b.histogram || [];
  $("#batch-win").textContent = "· " + WIN_LABEL[state.win];
  const tot = hist.reduce((s, x) => s + x.payment_count, 0) || 1;
  $("#batch-hist").innerHTML = hist.map(x => `
    <div class="kv"><span class="k">${x.bucket} payments/tx</span>
      <span class="v" style="display:flex;align-items:center;gap:8px">
        <span style="display:inline-block;height:8px;width:${Math.round(160 * x.payment_count / tot)}px;background:var(--agentic);border-radius:2px"></span>
        ${fmtCount(x.payment_count)} · ${pctOf(x.payment_count, tot)}</span></div>`).join("");
  $("#batch-kv").innerHTML = `
    <div class="kv"><span class="k">batched (≥2 in one tx)</span><span class="v c-cm">${(b.pct_batched * 100).toFixed(1)}%</span></div>
    <div class="kv"><span class="k">largest batch</span><span class="v">${fmtInt(b.max_batch_size)} payments</span></div>`;
}

function rWrapper() {
  const rows = win().selector_mix || [];
  $("#wrap-chip").textContent = `top ${rows.length} of N · not exhaustive`;
  const body = rows.map(r => {
    const lbl = selectorLabel(r.selector_hex);
    const name = lbl ? `${lbl} <span style="color:var(--faint)">0x${r.selector_hex}</span>` : `0x${r.selector_hex}`;
    return `<tr><td>${name}</td><td>${r.settlement_kind}</td><td>${fmtCount(r.txn_count)}</td></tr>`;
  }).join("");
  $("#wraptable").innerHTML = `<thead><tr><th>selector</th><th>kind</th><th>txns</th></tr></thead><tbody>${body || '<tr><td colspan="3">no selectors</td></tr>'}</tbody>`;
}

function rPhysics() {
  const a = win(), o = a.over_provisioning, wd = a.auth_window_width, bd = a.block_density;
  $("#phys-win").textContent = "· " + WIN_LABEL[state.win];
  $("#phys-kv").innerHTML = `
    <div class="kv"><span class="k">gas headroom (used/limit) p50</span><span class="v">${ratio(o.ratio_p50)} · p99 ${ratio(o.ratio_p99)}</span></div>
    <div class="kv"><span class="k">auth-window width p50 / p99</span><span class="v">${secs(wd.p50_s)} / ${secs(wd.p99_s)}</span></div>
    <div class="kv"><span class="k">payments/block max · mean</span><span class="v">${fmtInt(bd.max_per_block)} · ${bd.mean_per_block.toFixed(2)}</span></div>
    <div class="kv"><span class="k">distinct blocks</span><span class="v">${fmtCount(bd.distinct_blocks)}</span></div>`;
}

function rQA() {
  // hygiene is a GLOBAL canary — the rollup writes dup/replay only to the all/all
  // row, so always read it from the 'all' window. tx_value_nonzero is per-window.
  const a = win(), hy = view.windows.all.hygiene;
  $("#qa-kv").innerHTML = `
    <div class="kv"><span class="k">tx_value &gt; 0 (should be ~0)</span><span class="v">${fmtInt(a.tx_value_nonzero)}</span></div>
    <div class="kv"><span class="k">duplicate auth_nonce <span style="color:var(--faint)">(global)</span></span><span class="v">${fmtInt(hy.dup_auth_nonce)}</span></div>
    <div class="kv"><span class="k">same-block replays <span style="color:var(--faint)">(global)</span></span><span class="v">${fmtInt(hy.same_block_replay)}</span></div>`;
}

function rShell() {
  const errs = issues.filter(i => i.level === "error");
  const warns = issues.filter(i => i.level === "warn");
  const ok = errs.length === 0;
  $("#st-cons").textContent = ok ? "conservation ✓" : "conservation ✗";
  $("#st-cons").classList.toggle("bad", !ok);
  $("#st-gen").textContent = view.meta.generated_at;
  const a = view.windows.all;
  const lines = [
    `<div><span class="ps">$</span> <span class="cmd">fathom verify --mechanics --window all</span></div>`,
    `<div class="out">known+unknown == settlements · batch payments == settlements · pct_batched ∈ [0,1] <span class="ok">${ok ? "✓" : "✗"}</span></div>`,
    `<div class="out">${fmtCount(a.settlement_count)} settlements · ${pctOf(a.cost.breakeven_txn_count, a.settlement_count)} cost &gt; value · selector mix = top ${(a.selector_mix || []).length}, not exhaustive · tx_type 'other' = ${fmtInt(txTypeOther(a))}</div>`,
    ...errs.map(e => `<div class="out"><span class="bad">✗</span> ${e.msg}</div>`),
    ...warns.map(w => `<div class="out"><span class="warn">!</span> ${w.msg}</div>`),
    `<div class="out">every number from dist/mechanics.json · cost reused from the gas cube.<span class="cursor" style="margin-left:6px"></span></div>`,
  ];
  $("#shell").innerHTML = lines.join("");
}

const PIN = {
  economics() { const a = win(), c = a.cost; return { title: "UNIT ECONOMICS · " + state.win.toUpperCase(), value: pctOf(c.breakeven_txn_count, a.settlement_count) + " cost > value", context: `${fmtCount(c.breakeven_txn_count)} of ${fmtCount(a.settlement_count)} payments cost more gas than they moved · ${c.gas_cents_per_dollar} ¢/$ · ${num(c.gas_eth).toFixed(2)} ETH burned`, denom: "true cost = L2+L1 gas · " + WIN_LABEL[state.win] }; },
  fee() { const a = win(), t = a.fee.tx_type, n = a.settlement_count; return { title: "FEE INTENT · " + state.win.toUpperCase(), value: pctOf(t["2"] || 0, n) + " EIP-1559", context: `tx_type-2 ${pctOf(t["2"] || 0, n)}, legacy ${pctOf(t["0"] || 0, n)}, other ${fmtInt(txTypeOther(a))} · max_fee p50 ${gwei(a.fee.max_fee.p50)}`, denom: "tx_type mix · " + WIN_LABEL[state.win] }; },
  batch() { const b = win().batch; return { title: "BATCH MECHANICS · " + state.win.toUpperCase(), value: (b.pct_batched * 100).toFixed(1) + "% batched", context: `${(b.pct_batched * 100).toFixed(1)}% of payments share a tx (Multicall3 etc.); largest batch ${fmtInt(b.max_batch_size)}`, denom: "payments per tx · " + WIN_LABEL[state.win] }; },
  wrapper() { const r = (win().selector_mix || [])[0]; if (!r) return null; const lbl = selectorLabel(r.selector_hex) || ("0x" + r.selector_hex); return { title: "TOP WRAPPER · " + lbl, value: fmtCount(r.txn_count) + " settlements", context: `${lbl} (0x${r.selector_hex}, ${r.settlement_kind}) is the most-used settlement path`, denom: "top selector · " + WIN_LABEL[state.win] }; },
  physics() { const a = win(); return { title: "RAILS PHYSICS · " + state.win.toUpperCase(), value: "headroom p50 " + ratio(a.over_provisioning.ratio_p50), context: `gas used/limit p50 ${ratio(a.over_provisioning.ratio_p50)}; auth-window p50 ${secs(a.auth_window_width.p50_s)}; ${fmtInt(a.block_density.max_per_block)} payments in the busiest block`, denom: "rails physics · " + WIN_LABEL[state.win] }; },
  qa() { const a = win(), hy = view.windows.all.hygiene; return { title: "QA CANARIES · " + state.win.toUpperCase(), value: fmtInt(hy.dup_auth_nonce + hy.same_block_replay) + " anomalies", context: `${fmtInt(a.tx_value_nonzero)} tx carry ETH value, ${fmtInt(hy.dup_auth_nonce)} dup nonces (global), ${fmtInt(hy.same_block_replay)} same-block replays (global)`, denom: "data-quality canaries · hygiene is global" }; },
};

function renderAll() { rEconomics(); rFee(); rBatch(); rWrapper(); rPhysics(); rQA(); rShell(); }

function setWin(w) { state.win = w; $$("#winsel button").forEach(x => x.classList.toggle("on", x.dataset.win === w)); renderAll(); }

function wire() {
  $$("#winsel button").forEach(b => b.addEventListener("click", () => setWin(b.dataset.win)));
  $$(".pin-it").forEach(b => b.addEventListener("click", () => { const g = PIN[b.dataset.pin]; if (g) { const p = g(); if (p) tray.addPin(p); } }));
  $("#helptoggle").addEventListener("click", () => $("#help").classList.toggle("open"));
  $("#help").addEventListener("click", e => { if (e.target.id === "help") $("#help").classList.remove("open"); });
  document.addEventListener("keydown", e => {
    if (e.target.tagName === "TEXTAREA") return;
    if (e.key === "w") setWin(state.win === "all" ? "7d" : state.win === "7d" ? "30d" : "all");
    else if (e.key === "r") tray.toggle();
    else if (e.key === "?") $("#help").classList.toggle("open");
    else if (e.key === "Escape") { $("#help").classList.remove("open"); tray.toggle(false); }
  });
}

async function boot() {
  try {
    const res = await loadMechanics();
    view = res.view; issues = res.issues;
    $("#st-through").textContent = view.meta.data_through_day;
    $("#st-meth").textContent = "v" + view.meta.methodology_version;
    tray = createTray({ brand: "mechanics", meta: view.meta });
    tray.init();
    maybeGate(); wire(); renderAll();
  } catch (e) { fatal(e); }
}
boot();
