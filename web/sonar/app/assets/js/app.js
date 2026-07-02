// Boot + global wiring: gate, fetch, error screen, status bar, window
// switching, panel tools, keyboard, resize. Render functions live in
// charts.js / panels.js / tray.js.
import { $, $$ } from "./dom.js";
import { state, setData, setWinLabel } from "./state.js";
import { loadEconomy, winLabels } from "./adapter.js";
import { rDaily, rMonthly, rVelocity } from "./charts.js";
import { rOverview, rShape, rPrice, rGas, rClaims, rShell } from "./panels.js";
import { addPin, toggleTray, initTray, rCard } from "./tray.js";

/* ————— small-screen gate ————— */
const GATE_KEY = "fathom.smallScreenOk";
export function buildGateHtml(view){
  const w = view.windows.all;
  const payees = ((view.concentration.windows.all || {}).payee || {}).total_entities || 0;
  const payeesStr = payees >= 1e3 ? (payees/1e3).toFixed(0)+"k" : String(payees);
  return `
    <div>${(w.txn_count/1e6).toFixed(1)}M<small>PAYMENTS</small></div>
    <div>$${(parseFloat(w.volume_usdc)/1e6).toFixed(0)}M<small>VOLUME</small></div>
    <div>${payeesStr}<small>PAYEES</small></div>`;
}
function maybeGate(view){
  if (window.innerWidth >= 980 || localStorage.getItem(GATE_KEY) === "1") return;
  $("#gate-nums").innerHTML = buildGateHtml(view);
  $("#gate").classList.add("open");
  $("#gate-continue").addEventListener("click", () => {
    localStorage.setItem(GATE_KEY, "1");
    $("#gate").classList.remove("open");
  });
}

/* ————— error screen ————— */
function fatal(err){
  $("#fatal-msg").textContent = String(err && err.message || err);
  $("#fatal").classList.add("open");
}

/* ————— render orchestration ————— */
const WIN_PANELS = () => { rOverview(); rShape(); rPrice(); rGas(); rDaily(); };
const ALL_PANELS = () => { WIN_PANELS(); rMonthly(); rVelocity(); rClaims(); rShell(); };

function setWin(w){
  state.win = w;
  $$("#winsel button").forEach(x => x.classList.toggle("on", x.dataset.win === w));
  WIN_PANELS();
}

function wire(){
  $$("#winsel button").forEach(b => b.addEventListener("click", () => setWin(b.dataset.win)));
  $$(".ptools button[data-t]").forEach(b => {
    b.addEventListener("click", () => {
      const group = b.dataset.t;
      $$(`.ptools button[data-t="${group}"]`).forEach(x => x.classList.remove("on"));
      b.classList.add("on");
      if (group === "d-win")    state.dWin = b.dataset.v;
      if (group === "d-metric") state.dMetric = b.dataset.v === "usd" ? "usd" : "tx";
      if (group === "d-scale")  state.dScale = b.dataset.v;
      if (group === "d-ma")     state.dMa = b.dataset.v;
      if (group === "m-metric"){ state.mMetric = b.dataset.v; rMonthly(); return; }
      if (group === "b-metric"){ state.bMetric = b.dataset.v === "usd" ? "usd" : "tx"; rShape(); return; }
      rDaily();
    });
  });
  $$(".pin-it").forEach(b => b.addEventListener("click", () => addPin(b.dataset.pin)));
  $$(".panel").forEach(p => {
    p.addEventListener("mouseenter", () => {
      state.focused = p.id;
      $$(".panel").forEach(x => x.classList.toggle("focused", x === p));
    });
  });

  const help = $("#help");
  $("#helptoggle").addEventListener("click", () => help.classList.toggle("open"));
  help.addEventListener("click", e => { if (e.target === help) help.classList.remove("open"); });
  initTray();

  const PANEL_IDS = ["p1","p2","p3","p4","p5","p6","p7","p8","p9"];
  document.addEventListener("keydown", e => {
    if (e.metaKey || e.ctrlKey || e.altKey) return; // browser chords (Cmd+R, Ctrl+W…) are not ours
    if (e.target.tagName === "TEXTAREA" || e.target.tagName === "INPUT"){
      if (e.key === "Escape") e.target.blur();
      return;
    }
    if (e.key === "w"){ setWin(state.win === "7d" ? "30d" : state.win === "30d" ? "all" : "7d"); }
    if (e.key === "m"){
      state.dMetric = state.dMetric === "tx" ? "usd" : "tx";
      state.bMetric = state.dMetric; state.mMetric = state.dMetric === "tx" ? "tx" : "usd";
      $$('.ptools button[data-t$="-metric"]').forEach(b => {
        const on = (b.dataset.v === "usd") === (state.dMetric === "usd");
        b.classList.toggle("on", on);
      });
      rDaily(); rShape(); rMonthly();
    }
    if (e.key === "l"){ state.dScale = state.dScale === "lin" ? "log" : "lin";
      $$('.ptools button[data-t="d-scale"]').forEach(b => b.classList.toggle("on", b.dataset.v === state.dScale)); rDaily(); }
    if (e.key === "a"){ state.dMa = state.dMa === "raw" ? "ma7" : "raw";
      $$('.ptools button[data-t="d-ma"]').forEach(b => b.classList.toggle("on", b.dataset.v === state.dMa)); rDaily(); }
    if (e.key >= "1" && e.key <= "9"){
      const p = $("#"+PANEL_IDS[+e.key-1]);
      if (p){ p.scrollIntoView({behavior:"smooth", block:"center"});
        state.focused = p.id;
        $$(".panel").forEach(x => x.classList.toggle("focused", x === p)); }
    }
    if (e.key === "p"){
      const btn = $("#"+state.focused+" .pin-it");
      if (btn) addPin(btn.dataset.pin);
    }
    if (e.key === "r"){ toggleTray(); }
    if (e.key === "?"){ help.classList.toggle("open"); }
    if (e.key === "Escape"){ toggleTray(false); help.classList.remove("open"); }
  });

  let rsz;
  addEventListener("resize", () => { clearTimeout(rsz); rsz = setTimeout(() => { rDaily(); rVelocity(); }, 150); });
}

/* ————— status bar + banner from integrity results ————— */
function applyMeta(view, issues){
  $("#st-through").textContent = view.meta.data_through_day;
  $("#st-meth").textContent = "v" + view.meta.methodology_version;
  $("#st-gen").textContent = "emit " + view.meta.generated_at;
  const mon = d => new Date(d + "T00:00:00Z").toLocaleString("en-US", { month:"short", day:"2-digit", timeZone:"UTC" });
  // #d-range is owned by rDaily (reflects the selected tape slice); here we only
  // stamp the data edge used by the daily-tape denom.
  $("#d-edge").textContent = mon(view.meta.data_through_day);
  const errs = issues.filter(i => i.level === "error");
  const cons = $("#st-cons");
  cons.textContent = errs.length ? "conservation ✗" : "conservation ✓";
  if (errs.length) cons.style.color = "var(--contam)";
  if (issues.length){
    $("#banner").textContent = issues.map(i => i.msg).join(" · ");
    $("#banner").classList.add("open");
  }
}

/* ————— boot ————— */
(async function boot(){
  $("#fatal-retry").addEventListener("click", () => location.reload());
  let loaded;
  try { loaded = await loadEconomy(); }
  catch (err){ fatal(err); return; }
  const { view, issues } = loaded;
  setData(view);
  setWinLabel(winLabels(view));
  maybeGate(view);
  applyMeta(view, issues);
  wire();
  ALL_PANELS();
  if (location.hash === "#tray") toggleTray(true);
  if (document.fonts) document.fonts.ready.then(() => rCard());
})();
