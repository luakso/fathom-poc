// web/site/assets/js/reliability/app.js
// Boots /reliability: loads reliability.json (+ economy.json cross-check), renders
// overview, latency (validAfter→settle), window integrity, abandonment, cancellation
// attribution, daily trend, verify log. Mirrors payers/app.js.
import { $, $$ } from "../dom.js";
import { fmtCount, fmtInt } from "../format.js";
import { loadReliability, latencyBars } from "../lib/reliability-adapter.js";
import { shortAddr } from "../lib/leaderboard.js";
import { createTray } from "../lib/report-tray.js";

const WIN_LABEL = { "7d": "trailing 7d", "30d": "trailing 30d", "all": "all data" };
const state = { win: "all", attr: "by_payer" };
let view = null, issues = [], tray = null;

const GATE_KEY = "fathom.smallScreenOk";
function maybeGate() {
  if (window.innerWidth >= 980 || localStorage.getItem(GATE_KEY) === "1") return;
  const a = view.windows.all;
  $("#gate-nums").innerHTML = `
    <div>${fmtCount(a.settlement_count)}<small>SETTLEMENTS</small></div>
    <div>${ratePct(a.windowed_share)}<small>WINDOWED</small></div>
    <div>${fmtInt(a.cancellation_count)}<small>CANCELS</small></div>`;
  $("#gate").classList.add("open");
  $("#gate-continue").addEventListener("click", () => { localStorage.setItem(GATE_KEY, "1"); $("#gate").classList.remove("open"); });
}
function fatal(err) { $("#fatal-msg").textContent = String(err && err.message || err); $("#fatal").classList.add("open"); }
$("#fatal-retry").addEventListener("click", () => location.reload());

const win = () => view.windows[state.win];
function ratePct(r) { return (typeof r === "number" ? (r * 100).toFixed(2) : "0.00") + "%"; }
function secs(v) { return v == null ? "—" : (v < 1 ? v.toFixed(2) + "s" : Math.round(v) + "s"); }

function rOverview() {
  const a = win();
  $("#ov-win").textContent = "· " + WIN_LABEL[state.win];
  const outside = a.expired_count + a.not_yet_valid_count;
  $("#ov-stats").innerHTML = `
    <div class="bignum">${fmtCount(a.settlement_count)}<small>SETTLEMENTS</small></div>
    <div class="bignum">${ratePct(a.windowed_share)}<small>CARRY AUTH WINDOW</small></div>
    <div class="bignum c-cm glow">${ratePct(a.cancellation_rate)}<small>ABANDONMENT RATE</small></div>
    <div class="bignum">${fmtInt(outside)}<small>SETTLED OUTSIDE WINDOW</small></div>`;
  $("#ov-denom").textContent = "x402 settlements on Base · " + WIN_LABEL[state.win] + " · windowed = both auth bounds present";
}

function rLatency() {
  const l = win().latency;
  $("#lat-win").textContent = "· " + WIN_LABEL[state.win];
  $("#lat-caveat").textContent = "⚠ This is validAfter→settle, NOT true sign→settle. valid_after is set to a ≈10-min back-dated buffer at signing, so this measures the SDK/facilitator window convention plus block inclusion — not network settle delay.";
  $("#lat-stats").innerHTML = `
    <div class="bignum">${secs(l.p50_s)}<small>P50</small></div>
    <div class="bignum">${secs(l.p90_s)}<small>P90</small></div>
    <div class="bignum">${secs(l.p99_s)}<small>P99</small></div>`;
  const bars = latencyBars(l.buckets);
  $("#lat-hist").innerHTML = bars.map(b => `
    <div class="kv"><span class="k">${b.label}</span>
      <span class="v" style="display:flex;align-items:center;gap:8px">
        <span style="display:inline-block;height:8px;width:${Math.round(b.frac * 160)}px;background:var(--agentic);border-radius:2px"></span>
        ${fmtCount(b.count)} · ${(b.frac * 100).toFixed(1)}%</span></div>`).join("");
  $("#lat-denom").textContent = "histogram over the windowed subset · " + WIN_LABEL[state.win];
}

function rIntegrity() {
  const a = win();
  $("#wi-win").textContent = "· " + WIN_LABEL[state.win];
  $("#wi-kv").innerHTML = `
    <div class="kv"><span class="k">expired (settled after valid_before)</span><span class="v">${fmtInt(a.expired_count)} · ${ratePct(a.expired_rate)}</span></div>
    <div class="kv"><span class="k">not-yet-valid (settled before valid_after)</span><span class="v">${fmtInt(a.not_yet_valid_count)} · ${ratePct(a.not_yet_valid_rate)}</span></div>
    <div class="kv"><span class="k">windowed settlements (denominator)</span><span class="v">${fmtCount(a.windowed_count)}</span></div>`;
}

function rAbandon() {
  const a = win();
  $("#ab-win").textContent = "· " + WIN_LABEL[state.win];
  $("#ab-kv").innerHTML = `
    <div class="kv"><span class="k">cancellations</span><span class="v">${fmtInt(a.cancellation_count)}</span></div>
    <div class="kv"><span class="k">abandonment rate</span><span class="v c-cm">${ratePct(a.cancellation_rate)}</span></div>
    <div class="kv"><span class="k">settlements (denominator)</span><span class="v">${fmtCount(a.settlement_count)}</span></div>`;
}

function rAttribution() {
  const rows = view.attribution[state.attr] || [];
  $$("#attrsel button").forEach(x => x.classList.toggle("on", x.dataset.attr === state.attr));
  const body = rows.map(r => `<tr><td>${shortAddr(r.address)}</td><td>${fmtInt(r.count)}</td><td>${r.facilitator_known ? '<span class="c-ag">known</span>' : '<span class="c-cm">unknown</span>'}</td></tr>`).join("");
  $("#attrtable").innerHTML = `<thead><tr><th>address</th><th>cancellations</th><th>facilitator</th></tr></thead><tbody>${body || '<tr><td colspan="3">no cancellations in window</td></tr>'}</tbody>`;
}

function rDaily() {
  const d = view.daily;
  const host = $("#daily");
  if (!d.length) { host.innerHTML = '<div class="denom">no daily data</div>'; return; }
  const W = 720, H = 140, padL = 8, padR = 8, padT = 10, padB = 18;
  const max = Math.max(...d.map(x => x.settlement_count), 1);
  const x = i => padL + (W - padL - padR) * (d.length === 1 ? 0.5 : i / (d.length - 1));
  const y = v => H - padB - (H - padT - padB) * (v / max);
  const line = d.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)} ${y(p.settlement_count).toFixed(1)}`).join(" ");
  const area = line + ` L${x(d.length - 1).toFixed(1)} ${H - padB} L${x(0).toFixed(1)} ${H - padB} Z`;
  const marks = d.map((p, i) => p.cancellation_count > 0 ? `<circle cx="${x(i).toFixed(1)}" cy="${y(p.settlement_count).toFixed(1)}" r="3" fill="var(--accent)"/>` : "").join("");
  host.innerHTML = `<svg viewBox="0 0 ${W} ${H}" width="100%" preserveAspectRatio="none">
    <path d="${area}" fill="rgba(61,214,140,.10)"/>
    <path d="${line}" fill="none" stroke="var(--agentic)" stroke-width="1.5"/>
    ${marks}
    <line x1="${padL}" y1="${H - padB}" x2="${W - padR}" y2="${H - padB}" stroke="var(--line-2)"/>
    <text x="${padL}" y="${H - 4}" fill="var(--faint)" font-size="10">${d[0].day}</text>
    <text x="${W - padR}" y="${H - 4}" fill="var(--faint)" font-size="10" text-anchor="end">${d[d.length - 1].day}</text>
  </svg>`;
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
    `<div><span class="ps">$</span> <span class="cmd">fathom verify --reliability --window all</span></div>`,
    `<div class="out">windowed ≤ settlement · known+unknown == settlement · rates ∈ [0,1] <span class="ok">${ok ? "✓" : "✗"}</span></div>`,
    `<div class="out">${fmtCount(a.settlement_count)} settlements · ${ratePct(a.windowed_share)} windowed · ${fmtInt(a.cancellation_count)} cancellations</div>`,
    ...errs.map(e => `<div class="out"><span class="bad">✗</span> ${e.msg}</div>`),
    ...warns.map(w => `<div class="out"><span class="warn">!</span> ${w.msg}</div>`),
    `<div class="out">every number from dist/reliability.json · cross-checked vs economy.json.<span class="cursor" style="margin-left:6px"></span></div>`,
  ];
  $("#shell").innerHTML = lines.join("");
}

const PIN = {
  overview() { const a = win(); return { title: "RELIABILITY · " + state.win.toUpperCase(), value: ratePct(a.cancellation_rate) + " abandonment", context: `${fmtCount(a.settlement_count)} settlements · ${ratePct(a.windowed_share)} carry an auth window · ${a.expired_count + a.not_yet_valid_count} settled outside their signed window`, denom: "x402 on Base · " + WIN_LABEL[state.win] }; },
  latency() { const l = win().latency; return { title: "LATENCY · " + state.win.toUpperCase(), value: "p50 " + secs(l.p50_s), context: `validAfter→settle p50/p90/p99 = ${secs(l.p50_s)}/${secs(l.p90_s)}/${secs(l.p99_s)} — reflects the validAfter buffer convention, not true sign→settle`, denom: "windowed subset · " + WIN_LABEL[state.win] }; },
  integrity() { const a = win(); return { title: "WINDOW INTEGRITY · " + state.win.toUpperCase(), value: fmtInt(a.expired_count + a.not_yet_valid_count) + " outside window", context: `${a.expired_count} expired, ${a.not_yet_valid_count} not-yet-valid of ${fmtCount(a.windowed_count)} windowed settlements`, denom: "windowed subset · " + WIN_LABEL[state.win] }; },
  abandon() { const a = win(); return { title: "ABANDONMENT · " + state.win.toUpperCase(), value: ratePct(a.cancellation_rate), context: `${fmtInt(a.cancellation_count)} authorizations canceled before use across ${fmtCount(a.settlement_count)} settlements`, denom: "cancellations ÷ settlements · " + WIN_LABEL[state.win] }; },
  attr() { const r = (view.attribution[state.attr] || [])[0]; return r ? { title: "TOP CANCELLER · " + shortAddr(r.address), value: fmtInt(r.count) + " cancellations", context: `${state.attr === "by_payer" ? "payer" : "cancel submitter"} · facilitator ${r.facilitator_known ? "known" : "unknown"}`, denom: "from authorization_cancellation_v1" } : null; },
  daily() { const tot = view.daily.reduce((s, p) => s + p.cancellation_count, 0); return { title: "DAILY TREND", value: view.daily.length + " days", context: `${fmtCount(view.daily.reduce((s, p) => s + p.settlement_count, 0))} settlements, ${fmtInt(tot)} cancellations across the series`, denom: "full daily series · both memberships" }; },
};

function renderAll() { rOverview(); rLatency(); rIntegrity(); rAbandon(); rAttribution(); rDaily(); rShell(); }

function setWin(w) { state.win = w; $$("#winsel button").forEach(x => x.classList.toggle("on", x.dataset.win === w)); renderAll(); }
function setAttr(a) { state.attr = a; rAttribution(); }

function wire() {
  $$("#winsel button").forEach(b => b.addEventListener("click", () => setWin(b.dataset.win)));
  $$("#attrsel button").forEach(b => b.addEventListener("click", () => setAttr(b.dataset.attr)));
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
    const res = await loadReliability();
    view = res.view; issues = res.issues;
    $("#st-through").textContent = view.meta.data_through_day;
    $("#st-meth").textContent = "v" + view.meta.methodology_version;
    tray = createTray({ brand: "reliability", meta: view.meta });
    tray.init();
    maybeGate(); wire(); renderAll();
  } catch (e) { fatal(e); }
}
boot();
