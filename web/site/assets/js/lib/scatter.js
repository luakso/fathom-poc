// Fingerprint scatter: distinct counterparties (x, log) × distinct amounts
// (y, log), bubble area ∝ volume, color by fingerprint class. Native SVG
// <title> tooltips, so there are no listeners to leak. Role-agnostic: the
// caller supplies xLabel (payers for a payee page, payees for a payer page).
import { num, fmtMoney, fmtCount } from "../format.js";
import { classify, CLASSES } from "./fingerprint.js";
import { shortAddr } from "./leaderboard.js";

const log10 = v => Math.log10(Math.max(1, v));

export function classCounts(rows) {
  const t = {};
  for (const key of Object.keys(CLASSES)) t[key] = { count: 0, volume: 0 };
  for (const r of rows) { const k = classify(r); t[k].count++; t[k].volume += num(r.volume_usdc); }
  return t;
}

export function renderScatter(host, rows, opts) {
  const { xLabel } = opts;
  if (!rows.length) { host.innerHTML = `<div class="readout">no entities in this window</div>`; return; }
  const W = host.clientWidth || 700, H = 320, padL = 46, padB = 34, padT = 14, padR = 14;
  const pts = rows.map(r => ({
    r, cls: classify(r),
    x: r.distinct_counterparties, y: r.distinct_amounts, v: num(r.volume_usdc),
  }));
  const xMax = Math.max(1, ...pts.map(p => log10(p.x)));
  const yMax = Math.max(1, ...pts.map(p => log10(p.y)));
  const vMax = Math.max(1, ...pts.map(p => p.v));
  const px = x => padL + (W - padL - padR) * (log10(x) / xMax);
  const py = y => padT + (H - padT - padB) * (1 - log10(y) / yMax);
  const rad = v => 2.5 + 9 * Math.sqrt(v / vMax);

  const decade = (n, axis) => {
    let s = "";
    for (let e = 0; e <= n; e++) {
      const val = Math.pow(10, e);
      if (axis === "x") {
        const X = px(val);
        s += `<line class="gl" x1="${X}" y1="${padT}" x2="${X}" y2="${H - padB}"/><text x="${X + 3}" y="${H - padB + 14}">${fmtCount(val)}</text>`;
      } else {
        const Y = py(val);
        s += `<line class="gl" x1="${padL}" y1="${Y}" x2="${W - padR}" y2="${Y}"/><text x="${padL - 6}" y="${Y + 3}" text-anchor="end">${fmtCount(val)}</text>`;
      }
    }
    return s;
  };

  const circles = pts.map(p => {
    const m = CLASSES[p.cls];
    const tip = `${shortAddr(p.r.address)} · ${m.label}\n${fmtCount(p.r.txn_count)} tx · ${fmtMoney(p.r.volume_usdc)}\n${p.r.distinct_counterparties} ${xLabel} · ${p.r.distinct_amounts} amounts`;
    return `<circle cx="${px(p.x).toFixed(1)}" cy="${py(p.y).toFixed(1)}" r="${rad(p.v).toFixed(1)}" fill="${m.color}" fill-opacity="0.45" stroke="${m.color}" stroke-width="0.7"><title>${tip}</title></circle>`;
  }).join("");

  host.innerHTML = `<svg class="ch" viewBox="0 0 ${W} ${H}" height="${H}">
    ${decade(Math.ceil(yMax), "y")}${decade(Math.ceil(xMax), "x")}
    ${circles}
    <line class="ax" x1="${padL}" y1="${H - padB}" x2="${W - padR}" y2="${H - padB}"/>
    <text x="${W - padR}" y="${H - padB + 14}" text-anchor="end" style="fill:var(--faint)">distinct ${xLabel} →</text>
    <text x="${padL - 6}" y="${padT + 2}" text-anchor="end" style="fill:var(--faint)">amounts ↑</text>
  </svg>`;
}
