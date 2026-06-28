// web/site/assets/js/payees/app.js
// Boots /payees: loads payees.json, renders overview, fingerprint scatter +
// legend, leaderboard, dust-sink registry, concentration, verify log. Pins
// flow into the shared report tray.
import { $, $$ } from "../dom.js";
import { num, fmtMoney, fmtCount, fmtInt, fmtAmt, pct } from "../format.js";
import { loadEntity } from "../lib/entity-adapter.js";
import { renderLeaderboard, shortAddr } from "../lib/leaderboard.js";
import { renderScatter, classCounts } from "../lib/scatter.js";
import { classify, CLASSES } from "../lib/fingerprint.js";
import { createTray } from "../lib/report-tray.js";

const WIN_LABEL = { "7d": "trailing 7d", "30d": "trailing 30d", "all": "all data" };
const state = { win: "all" };
let view = null, issues = [], tray = null;

const GATE_KEY = "fathom.smallScreenOk";
function maybeGate() {
  if (window.innerWidth >= 980 || localStorage.getItem(GATE_KEY) === "1") return;
  const c = view.windows.all.concentration;
  $("#gate-nums").innerHTML = `
    <div>${fmtInt(c.total_entities)}<small>PAYEES</small></div>
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
  const top10$ = pct(c.top10_volume, c.total_volume);
  $("#ov-stats").innerHTML = `
    <div class="bignum">${fmtInt(c.total_entities)}<small>DISTINCT PAYEES</small></div>
    <div class="bignum">${fmtMoney(c.total_volume)}<small>VOLUME</small></div>
    <div class="bignum">${fmtCount(c.total_txns)}<small>PAYMENTS</small></div>
    <div class="bignum c-cm glow">${top10$}<small>TOP-10 SHARE OF $</small></div>`;
  $("#ov-denom").textContent = "x402 payees on Base · " + WIN_LABEL[state.win] + " · windows anchored to data-through day";
}

function rFingerprint() {
  const rows = win().leaderboard;
  renderScatter($("#scatter"), rows, { xLabel: "payers" });
  const t = classCounts(rows);
  $("#fp-legend").innerHTML = Object.entries(CLASSES).map(([k, m]) =>
    `<span class="lk"><i style="background:${m.color}"></i>${m.label} <b>${t[k].count}</b> · ${fmtMoney(t[k].volume)}</span>`).join("");
}

function rLeader() {
  $("#lb-win").textContent = "· " + WIN_LABEL[state.win];
  renderLeaderboard($("#lbtable"), win().leaderboard, {
    cpartyLabel: "payers",
    onPin: r => tray.addPin(pinForEntity(r)),
  });
}

function rSink() {
  const sinks = win().leaderboard.filter(r => classify(r) === "sink")
    .sort((a, b) => b.txn_count - a.txn_count).slice(0, 20);
  if (!sinks.length) { $("#sinktable").innerHTML = `<tbody><tr><td style="color:var(--faint);padding:14px 0">no dust sinks in this window</td></tr></tbody>`; return; }
  $("#sinktable").innerHTML = `<thead><tr><th>address</th><th>txns</th><th>$ absorbed</th><th>amounts</th></tr></thead>
    <tbody>${sinks.map(r => `<tr>
      <td style="color:var(--dim)" title="${r.address}">${shortAddr(r.address)}</td>
      <td style="font-weight:700">${fmtCount(r.txn_count)}</td>
      <td>${fmtMoney(r.volume_usdc)}</td><td>${fmtInt(r.distinct_amounts)}</td></tr>`).join("")}</tbody>`;
}

function rConc() {
  const c = win().concentration;
  $("#cn-win").textContent = "· " + WIN_LABEL[state.win];
  $("#conc-kv").innerHTML = `
    <div class="kv"><span class="k">top-10 share of $</span><span class="v c-cm">${pct(c.top10_volume, c.total_volume)}</span></div>
    <div class="kv"><span class="k">top-10 share of txns</span><span class="v">${pct(c.top10_txns, c.total_txns)}</span></div>
    <div class="kv"><span class="k">top-100 share of $</span><span class="v">${pct(c.top100_volume, c.total_volume)}</span></div>
    <div class="kv"><span class="k">distinct payees</span><span class="v">${fmtInt(c.total_entities)}</span></div>`;
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
    `<div class="out">distinct payees ${fmtInt(c.total_entities)} · ${fmtCount(c.total_txns)} payments · ${fmtMoney(c.total_volume)}</div>`,
    ...errs.map(e => `<div class="out"><span class="bad">✗</span> ${e.msg}</div>`),
    ...warns.map(w => `<div class="out"><span class="warn">!</span> ${w.msg}</div>`),
    `<div class="out">every entity from dist/payees.json — fingerprint classified, not curated.<span class="cursor" style="margin-left:6px"></span></div>`,
  ];
  $("#shell").innerHTML = lines.join("");
}

/* ————— pin builders ————— */
function pinForEntity(r) {
  const m = CLASSES[classify(r)];
  return {
    title: "PAYEE · " + shortAddr(r.address),
    value: fmtMoney(r.volume_usdc) + " · " + m.label,
    context: `${fmtCount(r.txn_count)} payments from ${fmtInt(r.distinct_counterparties)} payers across ${fmtInt(r.distinct_amounts)} amounts — ${m.blurb}`,
    denom: "x402 payees on Base · " + WIN_LABEL[state.win],
  };
}
const PIN = {
  overview() { const c = win().concentration; return { title: "PAYEES · " + state.win.toUpperCase(), value: fmtMoney(c.total_volume), context: `${fmtInt(c.total_entities)} distinct payees · ${fmtCount(c.total_txns)} payments · top-10 take ${pct(c.top10_volume, c.total_volume)} of $`, denom: "x402 payees on Base · " + WIN_LABEL[state.win] }; },
  fingerprint() { const t = classCounts(win().leaderboard); const tot = Object.values(t).reduce((s, x) => s + x.volume, 0) || 1; return { title: "FINGERPRINT · " + state.win.toUpperCase(), value: pct(t.service.volume, tot) + " is service $", context: `${t.service.count} services, ${t.sink.count} dust sinks, ${t.fleet.count} fleet targets, ${t.otc.count} OTC desks among the top leaderboard`, denom: "fingerprint over leaderboard · " + WIN_LABEL[state.win] }; },
  leader() { const r = win().leaderboard[0]; return r ? pinForEntity(r) : null; },
  sink() { const s = win().leaderboard.filter(r => classify(r) === "sink").sort((a, b) => b.txn_count - a.txn_count)[0]; if (!s) return null; const avg = s.txn_count ? num(s.volume_usdc) / s.txn_count : 0; return { title: "DUST SINK · " + shortAddr(s.address), value: fmtCount(s.txn_count) + " txns for " + fmtMoney(s.volume_usdc), context: `absorbs micro-spam across ${fmtInt(s.distinct_amounts)} amounts — averaging ${fmtAmt(avg.toFixed(6))} per payment`, denom: "fingerprint class = dust sink · " + WIN_LABEL[state.win] }; },
  conc() { const c = win().concentration; return { title: "CONCENTRATION · " + state.win.toUpperCase(), value: pct(c.top10_volume, c.total_volume) + " to top 10", context: `top-10 payees take ${pct(c.top10_volume, c.total_volume)} of $ and ${pct(c.top10_txns, c.total_txns)} of payments; top-100 take ${pct(c.top100_volume, c.total_volume)} of $`, denom: "concentration{} · cross-checked vs economy.json · " + WIN_LABEL[state.win] }; },
};

function renderAll() {
  rOverview(); rFingerprint(); rLeader(); rSink(); rConc(); rShell();
}

function setWin(w) {
  state.win = w;
  $$("#winsel button").forEach(x => x.classList.toggle("on", x.dataset.win === w));
  renderAll();
}

function wire() {
  $$("#winsel button").forEach(b => b.addEventListener("click", () => setWin(b.dataset.win)));
  $$(".pin-it").forEach(b => b.addEventListener("click", () => { const g = PIN[b.dataset.pin]; if (g) tray.addPin(g()); }));
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
    const res = await loadEntity("payee");
    view = res.view; issues = res.issues;
    $("#st-through").textContent = view.meta.data_through_day;
    $("#st-meth").textContent = "v" + view.meta.methodology_version;
    tray = createTray({ brand: "payees", meta: view.meta });
    tray.init();
    maybeGate(); wire(); renderAll();
  } catch (e) { fatal(e); }
}
boot();
