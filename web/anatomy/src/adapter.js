// Role-based lane layout. The dossier is a small, structured DAG, so instead of
// concentric rings (which overlap once cards have real width), nodes are placed
// in vertical lanes that read left→right as money flow:
//
//   facilitators │ payers │  TX + events  │ payees
//
// TX sits at the top of the center lane with its event nodes stacked beneath it;
// address nodes stack vertically within their role lane, centered on the events
// block. Lane pitch exceeds card width and the row gap exceeds card height, so
// nodes never overlap. Positions are deterministic (index-based, no randomness).

// Approximate rendered card sizes (kept in sync with the widths in theme.css).
// Exported so layout invariants (no overlap) can be asserted in tests.
export const NODE_SIZE = {
  transaction: { w: 260, h: 190 },
  event: { w: 200, h: 96 },
  address: { w: 220, h: 110 },
}

// Lane x-centers. Order encodes the left→right reading.
const LANE_X = { facilitator: -860, payer: -440, center: 0, payee: 440 }
const ROW = 170 // vertical pitch between stacked nodes
const EVENTS_TOP = 270 // y-center of the first event, below the TX node

// laneOf picks one lane for a (possibly multi-role) address. A money role wins
// over the facilitator role so the left→right payer→payee flow stays legible;
// pure facilitators get their own lane.
function laneOf(node) {
  const roles = node.roles || []
  if (roles.includes('payer')) return 'payer'
  if (roles.includes('payee')) return 'payee'
  return 'facilitator'
}

// topLeft converts a desired center point to a React Flow top-left position.
function topLeft(cx, cy, kind) {
  const s = NODE_SIZE[kind] || NODE_SIZE.address
  return { x: Math.round(cx - s.w / 2), y: Math.round(cy - s.h / 2) }
}

// graphToFlow maps the API dossier graph to React Flow nodes/edges using the
// role-based lane layout described above.
export function graphToFlow(graph) {
  const events = graph.nodes.filter((n) => n.kind === 'event')
  const lanes = { facilitator: [], payer: [], payee: [] }
  for (const n of graph.nodes) {
    if (n.kind === 'address') lanes[laneOf(n)].push(n)
  }

  // Vertical center of the center lane's event block; address lanes center on it.
  const centerY = events.length ? EVENTS_TOP + ((events.length - 1) * ROW) / 2 : 0

  const pos = {}
  for (const n of graph.nodes) {
    if (n.kind === 'transaction') pos[n.id] = topLeft(LANE_X.center, 0, 'transaction')
  }
  events.forEach((e, i) => {
    pos[e.id] = topLeft(LANE_X.center, EVENTS_TOP + i * ROW, 'event')
  })
  for (const lane of ['facilitator', 'payer', 'payee']) {
    const list = lanes[lane]
    list.forEach((a, j) => {
      const cy = centerY + (j - (list.length - 1) / 2) * ROW
      pos[a.id] = topLeft(LANE_X[lane], cy, 'address')
    })
  }

  const nodes = graph.nodes.map((n) => ({
    id: n.id,
    type: n.kind,
    position: pos[n.id] || { x: 0, y: 0 },
    data: n,
  }))
  const edges = graph.edges.map((e) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    label: e.label || e.kind,
    animated: false,
  }))
  return { nodes, edges }
}

// applyStats returns a new flow with stats merged into the node `addrId`'s data,
// without mutating the input (immutable update).
export function applyStats(flow, addrId, stats) {
  return {
    ...flow,
    nodes: flow.nodes.map((n) =>
      n.id === addrId ? { ...n, data: { ...n.data, stats } } : n,
    ),
  }
}
