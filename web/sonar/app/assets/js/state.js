// Shared mutable UI state + the loaded dataset. ES-module live bindings let
// every renderer see updates without threading arguments.
// dWin is the daily tape's OWN timeframe (independent of the data-backed global
// `win`); it slices the full daily series client-side, default 30d.
export const state = { win:"all", dWin:"30d", dMetric:"tx", dScale:"lin", dMa:"raw", mMetric:"usd", bMetric:"tx", focused:"p1" };

export let data = null;
export function setData(d){ data = d; }

// Window labels; "all" is derived from the data's month range by the adapter.
export let winLabel = { "7d":"trailing 7d", "30d":"trailing 30d", "all":"all data" };
export function setWinLabel(l){ winLabel = l; }

// Integrity check results from checkIntegrity(); consumed by rShell.
export let issues = [];
export function setIssues(i){ issues = i; }

// Secondary facilitators artifact; null when the fetch failed or hasn't run yet.
// rFacilitators renders an absent-state when this is null.
export let facData = null;
export function setFacData(d){ facData = d; }
