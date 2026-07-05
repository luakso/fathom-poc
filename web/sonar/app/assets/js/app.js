// Boot + global wiring: gate, fetch, error screen, status bar, window
// switching, panel tools, keyboard, resize. Render functions live in
// charts.js / panels.js / tray.js.
import { $, $$ } from "./dom.js";
import { state, setData, setWinLabel, setIssues, setFacData } from "./state.js";
import { loadEconomy, loadFacilitators, winLabels } from "./adapter.js";
import { rDaily, rMonthly, rVelocity, rActiveWallets, rGasCostDaily } from "./charts.js";
import { rOverview, rShape, rPrice, rGas, rClaims, rPayerCohorts, rShell, rFacilitators } from "./panels.js";
import { addPin, toggleTray, initTray, rCard } from "./tray.js";

/* ————— small-screen gate ————— */
const GATE_KEY = "fathom.smallScreenOk";
let _view = null; // stored at boot for use in resize handler
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
  if (localStorage.getItem(GATE_KEY) === "1"){ $("#gate").classList.remove("open"); return; }
  if (window.innerWidth < 980){
    if (!$("#gate").classList.contains("open")){
      $("#gate-nums").innerHTML = buildGateHtml(view);
      $("#gate").classList.add("open");
    }
  } else {
    $("#gate").classList.remove("open");
  }
}

/* ————— error screen ————— */
function fatal(err){
  // Clear loading indicator if still visible
  const cons = document.getElementById("st-cons");
  if (cons && cons.textContent.includes("loading")) cons.textContent = "";
  $("#fatal-msg").textContent = String(err && err.message || err);
  $("#fatal").classList.add("open");
}

/* ————— render orchestration ————— */
/* WIN_PANELS re-render on window switch; the rest render once per data load
   (rPayerCohorts/rActiveWallets draw all windows at once — if one ever becomes
   window-sensitive, move it into WIN_PANELS). */
const WIN_PANELS = () => { rOverview(); rShape(); rPrice(); rGas(); rDaily(); };
const ALL_PANELS = () => { WIN_PANELS(); rMonthly(); rVelocity(); rClaims(); rActiveWallets(); rGasCostDaily(); rPayerCohorts(); rFacilitators(); rShell(); };

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

  const PANEL_IDS = ["p1","p2","p3","p5","p6","p7","p8","p9","p11"];
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
    if (e.key === "0"){
      const p = $("#p12");
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
  addEventListener("resize", () => { clearTimeout(rsz); rsz = setTimeout(() => { rDaily(); rVelocity(); rActiveWallets(); rGasCostDaily(); maybeGate(_view); }, 150); });
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
  const errs    = issues.filter(i => !i.pass && i.level === "error");
  const failing = issues.filter(i => !i.pass);
  const cons = $("#st-cons");
  cons.textContent = errs.length ? "conservation ✗" : "conservation ✓";
  if (errs.length) cons.style.color = "var(--contam)";
  if (failing.length){
    const shown = failing.slice(0, 3);
    const extra = failing.length - 3;
    const msgs = shown.map(i => i.detail || i.msg).join(" · ");
    const moreStr = extra > 0 ? ` …${extra} more` : "";
    const bannerEl = $("#banner");
    bannerEl.textContent = msgs + moreStr;
    const dismiss = document.createElement("button");
    dismiss.id = "banner-dismiss";
    dismiss.textContent = "×";
    dismiss.style.cssText = "margin-left:8px;background:transparent;border:none;color:inherit;cursor:pointer;font:inherit";
    dismiss.addEventListener("click", () => bannerEl.classList.remove("open"));
    bannerEl.appendChild(dismiss);
    bannerEl.classList.add("open");
  }
}

/* ————— boot ————— */
(async function boot(){
  $("#fatal-retry").addEventListener("click", () => location.reload());
  // Show a terminal-styled loading indicator while the artifact is fetched and
  // parsed. applyMeta() overwrites it on success; fatal() clears it on error.
  const cons = document.getElementById("st-cons");
  if (cons) cons.textContent = "loading verified payments...";

  // economy.json is critical (fatal on failure); facilitators.json is optional
  // — a failed fetch renders an absent-state in panel 10 but does not block boot.
  const [econResult, facResult] = await Promise.allSettled([
    loadEconomy(),
    loadFacilitators(),
  ]);

  if (econResult.status === "rejected"){ fatal(econResult.reason); return; }
  const { view, issues } = econResult.value;

  setData(view);
  setIssues(issues);
  setWinLabel(winLabels(view));
  // facData stays null if the facilitators fetch failed — rFacilitators shows
  // an absent-state.  No fatal call; the rest of the page is fully functional.
  if (facResult.status === "fulfilled") setFacData(facResult.value);

  _view = view;
  maybeGate(view);
  // gate-continue: set opt-out flag. Registered once at boot; handler is
  // idempotent so it works correctly after the gate is re-enabled via the
  // help overlay (which clears the flag and allows the gate to reopen).
  $("#gate-continue").addEventListener("click", () => {
    localStorage.setItem(GATE_KEY, "1");
    $("#gate").classList.remove("open");
  });
  // gate-reenable: clear opt-out flag so next resize re-evaluates the gate.
  const reenableEl = document.getElementById("gate-reenable");
  if (reenableEl) {
    reenableEl.addEventListener("click", e => {
      e.preventDefault();
      localStorage.removeItem(GATE_KEY);
    });
  }
  applyMeta(view, issues);
  wire();
  ALL_PANELS();
  if (location.hash === "#tray") toggleTray(true);
  if (document.fonts) document.fonts.ready.then(() => rCard());
})();
