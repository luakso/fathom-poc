// web/site/assets/js/payers/app.js
// Boots /payers: loads payers.json, renders overview, activity buckets,
// leaderboard + skew, concentration, demand-side fingerprint, verify log.
import { $, $$ } from "../dom.js";
import { fmtMoney, fmtCount, fmtInt, pct } from "../format.js";
import { loadEntity } from "../lib/entity-adapter.js";
import { renderLeaderboard, shortAddr } from "../lib/leaderboard.js";
import { renderScatter, classCounts } from "../lib/scatter.js";
import { CLASSES } from "../lib/fingerprint.js";
import { renderBuckets } from "../lib/buckets.js";
import { createTray } from "../lib/report-tray.js";

const WIN_LABEL = { "7d": "trailing 7d", "30d": "trailing 30d", "all": "all data" };
const state = { win: "all", bMetric: "count" };
let view = null, issues = [], tray = null;

const GATE_KEY = "fathom.smallScreenOk";
function maybeGate() {
  if (window.innerWidth >= 980 || localStorage.getItem(GATE_KEY) === "1") return;
  const c = view.windows.all.concentration;
  $("#gate-nums").innerHTML = `
    <div>${fmtInt(c.total_entities)}<small>PAYERS</small></div>
    <div>${fmtMoney(c.total_volume)}<small>VOLUME</small></div>
    <div>${fmtCount(c.total_txns)}<small>PAYMENTS</small></div>`;
  $("#gate").classList.add("open");
  $("#gate-continue").addEventListener("click", () => { localStorage.setItem(GATE_KEY, "1"); $("#gate").classList.remove("open"); });
}
function fatal(err) { $("#fatal-msg").textContent = String(err && err.message || err); $("#fatal").classList.add("open"); }
$("#fatal-retry").addEventListener("click", () => location.reload());

const win = () => view.windows[state.win];

function rOverview() {
  const c = win().concentration;
  $("#ov-win").textContent = "· " + WIN_LABEL[state.win];
  $("#ov-stats").innerHTML = `
    <div class="bignum">${fmtInt(c.total_entities)}<small>DISTINCT PAYERS</small></div>
    <div class="bignum">${fmtMoney(c.total_volume)}<small>VOLUME</small></div>
    <div class="bignum">${fmtCount(c.total_txns)}<small>PAYMENTS</small></div>
    <div class="bignum c-cm glow">${pct(c.top10_volume, c.total_volume)}<small>TOP-10 SHARE OF $</small></div>`;
  $("#ov-denom").textContent = "x402 payers on Base · " + WIN_LABEL[state.win] + " · windows anchored to data-through day";
}

function rBuckets() {
  renderBuckets($("#buckets"), win().buckets, { metric: state.bMetric });
}

function rLeader() {
  $("#lb-win").textContent = "· " + WIN_LABEL[state.win];
  const c = win().concentration;
  $("#skew").innerHTML = `<b>spend skew:</b> the top 10 payers drive <span class="c-cm">${pct(c.top10_volume, c.total_volume)}</span> of $ and <span class="c-cm">${pct(c.top10_txns, c.total_txns)}</span> of payments across ${fmtInt(c.total_entities)} wallets.`;
  renderLeaderboard($("#lbtable"), win().leaderboard, { cpartyLabel: "payees", onPin: r => tray.addPin(pinForEntity(r)) });
}

function rConc() {
  const c = win().concentration;
  $("#cn-win").textContent = "· " + WIN_LABEL[state.win];
  $("#conc-kv").innerHTML = `
    <div class="kv"><span class="k">top-10 share of $</span><span class="v c-cm">${pct(c.top10_volume, c.total_volume)}</span></div>
    <div class="kv"><span class="k">top-10 share of txns</span><span class="v">${pct(c.top10_txns, c.total_txns)}</span></div>
    <div class="kv"><span class="k">top-100 share of $</span><span class="v">${pct(c.top100_volume, c.total_volume)}</span></div>
    <div class="kv"><span class="k">distinct payers</span><span class="v">${fmtInt(c.total_entities)}</span></div>`;
}

function rFingerprint() {
  const rows = win().leaderboard;
  renderScatter($("#scatter"), rows, { xLabel: "payees" });
  const t = classCounts(rows);
  $("#fp-legend").innerHTML = Object.entries(CLASSES).map(([k, m]) =>
    `<span class="lk"><i style="background:${m.color}"></i>${m.label} <b>${t[k].count}</b> · ${fmtMoney(t[k].volume)}</span>`).join("");
}

function rShell() {
  const errs = issues.filter(i => i.level === "error");
  const warns = issues.filter(i => i.level === "warn");
  const ok = errs.length === 0;
  $("#st-cons").textContent = ok ? "conservation ✓" : "conservation ✗";
  $("#st-cons").classList.toggle("bad", !ok);
  $("#st-gen").textContent = view.meta.generated_at;
  const c = view.windows.all.concentration;
  const lines = [
    `<div><span class="ps">$</span> <span class="cmd">fathom verify --conservation --window all</span></div>`,
    `<div class="out">buckets reconcile to concentration totals <span class="ok">${ok ? "✓" : "✗"}</span></div>`,
    `<div class="out">distinct payers ${fmtInt(c.total_entities)} · ${fmtCount(c.total_txns)} payments · ${fmtMoney(c.total_volume)}</div>`,
    ...errs.map(e => `<div class="out"><span class="bad">✗</span> ${e.msg}</div>`),
    ...warns.map(w => `<div class="out"><span class="warn">!</span> ${w.msg}</div>`),
    `<div class="out">every agent from dist/payers.json — fingerprint classified, not curated.<span class="cursor" style="margin-left:6px"></span></div>`,
  ];
  $("#shell").innerHTML = lines.join("");
}

function pinForEntity(r) {
  return {
    title: "PAYER · " + shortAddr(r.address),
    value: fmtMoney(r.volume_usdc) + " spent",
    context: `${fmtCount(r.txn_count)} payments to ${fmtInt(r.distinct_counterparties)} payees across ${fmtInt(r.distinct_amounts)} amounts`,
    denom: "x402 payers on Base · " + WIN_LABEL[state.win],
  };
}
const PIN = {
  overview() { const c = win().concentration; return { title: "PAYERS · " + state.win.toUpperCase(), value: fmtMoney(c.total_volume), context: `${fmtInt(c.total_entities)} distinct payers · ${fmtCount(c.total_txns)} payments · top-10 drive ${pct(c.top10_volume, c.total_volume)} of $`, denom: "x402 payers on Base · " + WIN_LABEL[state.win] }; },
  buckets() { const b = Object.fromEntries(win().buckets.map(x => [x.bucket, x])); const one = b["1"] || { entity_count: 0 }; const tot = win().concentration.total_entities || 1; return { title: "ACTIVITY · " + state.win.toUpperCase(), value: pct(one.entity_count, tot) + " are one-shot", context: `${fmtInt(one.entity_count)} of ${fmtInt(tot)} payers made exactly one payment — testers, not recurring agents`, denom: "lifetime txn-count buckets · " + WIN_LABEL[state.win] }; },
  leader() { const r = win().leaderboard[0]; return r ? pinForEntity(r) : null; },
  conc() { const c = win().concentration; return { title: "CONCENTRATION · " + state.win.toUpperCase(), value: pct(c.top10_volume, c.total_volume) + " to top 10", context: `top-10 payers drive ${pct(c.top10_volume, c.total_volume)} of $ and ${pct(c.top10_txns, c.total_txns)} of payments; top-100 drive ${pct(c.top100_volume, c.total_volume)} of $`, denom: "concentration{} · cross-checked vs economy.json · " + WIN_LABEL[state.win] }; },
  fingerprint() { const t = classCounts(win().leaderboard); return { title: "FINGERPRINT · " + state.win.toUpperCase(), value: `${t.service.count} service-shaped payers`, context: `${t.service.count} broad spenders, ${t.fleet.count} fleet-shaped, ${t.otc.count} OTC-shaped among the top leaderboard`, denom: "fingerprint over leaderboard · " + WIN_LABEL[state.win] }; },
};

function renderAll() { rOverview(); rBuckets(); rLeader(); rConc(); rFingerprint(); rShell(); }

function setWin(w) { state.win = w; $$("#winsel button").forEach(x => x.classList.toggle("on", x.dataset.win === w)); renderAll(); }
function setMetric(v) { state.bMetric = v; $$('[data-t="b-metric"]').forEach(x => x.classList.toggle("on", x.dataset.v === v)); rBuckets(); }

function wire() {
  $$("#winsel button").forEach(b => b.addEventListener("click", () => setWin(b.dataset.win)));
  $$('[data-t="b-metric"]').forEach(b => b.addEventListener("click", () => setMetric(b.dataset.v)));
  $$(".pin-it").forEach(b => b.addEventListener("click", () => { const g = PIN[b.dataset.pin]; if (g) tray.addPin(g()); }));
  $("#helptoggle").addEventListener("click", () => $("#help").classList.toggle("open"));
  $("#help").addEventListener("click", e => { if (e.target.id === "help") $("#help").classList.remove("open"); });
  document.addEventListener("keydown", e => {
    if (e.target.tagName === "TEXTAREA") return;
    if (e.key === "w") setWin(state.win === "all" ? "7d" : state.win === "7d" ? "30d" : "all");
    else if (e.key === "m") setMetric(state.bMetric === "count" ? "txns" : state.bMetric === "txns" ? "usd" : "count");
    else if (e.key === "r") tray.toggle();
    else if (e.key === "?") $("#help").classList.toggle("open");
    else if (e.key === "Escape") { $("#help").classList.remove("open"); tray.toggle(false); }
  });
}

async function boot() {
  try {
    const res = await loadEntity("payer");
    view = res.view; issues = res.issues;
    $("#st-through").textContent = view.meta.data_through_day;
    $("#st-meth").textContent = "v" + view.meta.methodology_version;
    tray = createTray({ brand: "payers", meta: view.meta });
    tray.init();
    maybeGate(); wire(); renderAll();
  } catch (e) { fatal(e); }
}
boot();
