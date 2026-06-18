const RING = { event: 220, address: 420 }

// graphToFlow maps the API dossier graph to React Flow nodes/edges with a
// deterministic radial layout: tx at center, events on an inner ring, addresses
// on an outer ring, each placed by its index within its kind.
export function graphToFlow(graph) {
  const byKind = { event: [], address: [] }
  for (const n of graph.nodes) {
    if (n.kind === 'event' || n.kind === 'address') byKind[n.kind].push(n.id)
  }
  const angle = (kind, id) => {
    const list = byKind[kind]
    const i = list.indexOf(id)
    return (2 * Math.PI * i) / Math.max(list.length, 1)
  }
  const nodes = graph.nodes.map((n) => {
    let position = { x: 0, y: 0 }
    if (n.kind === 'event' || n.kind === 'address') {
      const r = RING[n.kind]
      const a = angle(n.kind, n.id)
      position = { x: Math.round(r * Math.cos(a)), y: Math.round(r * Math.sin(a)) }
    }
    return { id: n.id, type: n.kind, position, data: n }
  })
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
