// Activity buckets: payers (or payees) grouped by lifetime txn count. Bars in
// canonical order so the shape reads left-to-right regardless of artifact key
// ordering. Reuses the .mrow/.meter/.bar idiom.
import { num, fmtCount, fmtMoney, fmtInt } from "../format.js";

export const BUCKET_ORDER = ["1", "2-10", "11-100", "101-1k", "1k-100k", "100k+"];

export function renderBuckets(host, buckets, opts) {
  const metric = opts.metric || "count";
  const byKey = Object.fromEntries(buckets.map(b => [b.bucket, b]));
  const ZERO = { entity_count: 0, txn_sum: 0, volume_sum: "0" };
  const get = b => metric === "usd" ? num(b.volume_sum) : metric === "txns" ? b.txn_sum : b.entity_count;
  const max = Math.max(1, ...BUCKET_ORDER.map(k => get(byKey[k] || ZERO)));
  host.innerHTML = BUCKET_ORDER.map(k => {
    const b = byKey[k] || ZERO;
    const v = get(b), wp = Math.max(1, 66 * v / max);
    const main = metric === "usd" ? fmtMoney(v) : metric === "txns" ? fmtCount(v) + " tx" : fmtInt(v) + " wallets";
    const sub = `${fmtInt(b.entity_count)} wallets · ${fmtCount(b.txn_sum)} tx · ${fmtMoney(b.volume_sum)}`;
    return `<div class="mrow">
      <span class="lab">${k}<small>txns</small></span>
      <span class="meter"><span class="bar ${k === "1" || k === "2-10" ? "dim" : ""}" style="width:${wp}%"></span>
      <span class="val">${main}</span><span class="sub">${sub}</span></span>
    </div>`;
  }).join("");
}
