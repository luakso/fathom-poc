// Sortable entity leaderboard. Re-renders its own <table> on header click;
// innerHTML replacement means click listeners die with their nodes (no leak).
import { num, fmtMoney, fmtInt, fmtCount, pct } from "../format.js";
import { classify, CLASSES } from "./fingerprint.js";

export const shortAddr = a => a.slice(0, 6) + "…" + a.slice(-4);
const day = s => String(s).slice(0, 10);

// column id -> accessor for sorting
const SORT = {
  volume: r => num(r.volume_usdc),
  txns: r => r.txn_count,
  cparties: r => r.distinct_counterparties,
  amounts: r => r.distinct_amounts,
};

export function renderLeaderboard(tableEl, rows, opts) {
  const { cpartyLabel, limit = 100, onPin } = opts;
  const stateKey = "_lb";
  if (!tableEl[stateKey]) tableEl[stateKey] = { sort: "volume", dir: -1 };
  const st = tableEl[stateKey];

  function draw() {
    if (!rows.length) {
      tableEl.innerHTML = `<tbody><tr><td style="color:var(--faint);padding:14px 0">no entities in this window</td></tr></tbody>`;
      return;
    }
    const sorted = [...rows].sort((a, b) => st.dir * (SORT[st.sort](a) - SORT[st.sort](b))).slice(0, limit);
    const arrow = id => st.sort === id ? (st.dir < 0 ? " ▾" : " ▴") : "";
    const head = `<thead><tr>
      <th>#</th>
      <th data-sort="volume" style="cursor:pointer">$ volume${arrow("volume")}</th>
      <th data-sort="txns" style="cursor:pointer">txns${arrow("txns")}</th>
      <th data-sort="cparties" style="cursor:pointer">${cpartyLabel}${arrow("cparties")}</th>
      <th data-sort="amounts" style="cursor:pointer">amounts${arrow("amounts")}</th>
      <th>known%</th><th>fingerprint</th><th>address</th><th></th>
    </tr></thead>`;
    const body = `<tbody>${sorted.map((r, i) => {
      const cls = classify(r);
      const meta = CLASSES[cls];
      const known = pct(r.known_volume_usdc, r.volume_usdc);
      return `<tr>
        <td style="color:var(--faint)">${i + 1}</td>
        <td style="font-weight:700">${fmtMoney(r.volume_usdc)}</td>
        <td>${fmtInt(r.txn_count)}</td>
        <td>${fmtInt(r.distinct_counterparties)}</td>
        <td>${fmtInt(r.distinct_amounts)}</td>
        <td style="color:var(--dim)">${known}</td>
        <td><span class="fp" style="color:${meta.color}" title="${meta.blurb}">${meta.label}</span></td>
        <td title="${r.address}\n${day(r.first_seen)} → ${day(r.last_seen)}" style="color:var(--dim)">${shortAddr(r.address)}</td>
        <td>${onPin ? `<button class="pin-row" data-pin-row="${i}" title="pin this entity">⊞</button>` : ""}</td>
      </tr>`;
    }).join("")}</tbody>`;
    tableEl.innerHTML = head + body;

    tableEl.querySelectorAll("thead th[data-sort]").forEach(th => th.addEventListener("click", () => {
      const id = th.dataset.sort;
      if (st.sort === id) st.dir *= -1; else { st.sort = id; st.dir = -1; }
      draw();
    }));
    if (onPin) tableEl.querySelectorAll("[data-pin-row]").forEach(b => b.addEventListener("click", () => {
      onPin(sorted[+b.dataset.pinRow]);
    }));
  }
  draw();
}
