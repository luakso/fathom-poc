// Shared mutable UI state + the loaded dataset. ES-module live bindings let
// every renderer see updates without threading arguments.
export const state = { win:"all", dMetric:"tx", dScale:"lin", dMa:"raw", mMetric:"usd", bMetric:"tx", focused:"p1" };

export let data = null;
export function setData(d){ data = d; }

// Window labels; "all" is derived from the data's month range by the adapter.
export let winLabel = { "7d":"trailing 7d", "30d":"trailing 30d", "all":"all data" };
export function setWinLabel(l){ winLabel = l; }
