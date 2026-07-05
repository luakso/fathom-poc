// Entity canvas state. Pure functions: build the initial subject graph,
// merge an expansion, derive out-of-lens ghosts. Node id = address, so the
// graph dedupes by construction (spec §2.2).
import type { Node, Edge } from '@xyflow/react'
import {
  NODE_W, NODE_H, GHOST_W, GHOST_H, COL_GAP,
  placeColumn, columnX, type Rect, type Dir,
} from './layout'
import { usd, groupDigits, pct } from '../lib/format'
import type { Neighbors, NeighborRow } from '../lib/schemas'

export type EntityNodeData = {
  address: string
  label?: string
  roleClass: 'subject' | 'payee' | 'payer' | 'facilitator' | 'ghost'
  chips: string[]
  kpis?: { payments: string; volume: string; counterparties: string }
  detail?: string
  expandable: boolean
  [key: string]: unknown
}

export type FlowState = { nodes: Node<EntityNodeData>[]; edges: Edge[] }

const SUBJECT_H = NODE_H + 40

function nodeRect(n: Node<EntityNodeData>): Rect {
  const ghost = n.type === 'ghost'
  return {
    x: n.position.x,
    y: n.position.y,
    w: ghost ? GHOST_W : NODE_W,
    h: ghost ? GHOST_H : n.data.roleClass === 'subject' ? SUBJECT_H : NODE_H,
  }
}

function flowLabel(r: NeighborRow): string {
  return `${usd(r.volumeUsdc)} · ${groupDigits(r.txnCount)} txns · ${pct(r.share)}`
}

function makeNode(
  r: NeighborRow,
  roleClass: EntityNodeData['roleClass'],
  rect: Rect,
  ghost: boolean,
): Node<EntityNodeData> {
  return {
    id: r.address,
    type: ghost ? 'ghost' : 'entity',
    position: { x: rect.x, y: rect.y },
    data: {
      address: r.address,
      label: r.label,
      roleClass,
      chips: ghost ? ['outside lens'] : [roleClass],
      expandable: !ghost,
    },
  }
}

function makeEdge(source: string, target: string, r: NeighborRow, opts: { settles?: boolean; ghost?: boolean }): Edge {
  if (opts.settles) {
    return { id: `${source}=>${target}`, source, target, type: 'flow', data: { label: 'settles', verb: true } }
  }
  return {
    id: `${source}->${target}`,
    source,
    target,
    type: 'flow',
    data: { share: Number(r.share), label: flowLabel(r), primary: false, ghost: Boolean(opts.ghost) },
  }
}

type Direction = { rows: NeighborRow[]; dir: Dir; toAnchor: boolean; settles: boolean; roleClass: EntityNodeData['roleClass'] }

function directions(n: Neighbors): Direction[] {
  return [
    { rows: n.payers?.rows ?? [], dir: 'left', toAnchor: true, settles: false, roleClass: 'payer' },
    { rows: n.payees?.rows ?? [], dir: 'right', toAnchor: false, settles: false, roleClass: 'payee' },
    { rows: n.facilitators?.rows ?? [], dir: 'up', toAnchor: true, settles: true, roleClass: 'facilitator' },
    { rows: n.settledPayers?.rows ?? [], dir: 'left', toAnchor: true, settles: true, roleClass: 'payer' },
    { rows: n.settledPayees?.rows ?? [], dir: 'right', toAnchor: false, settles: true, roleClass: 'payee' },
  ]
}

// merge adds one anchor's neighbor directions into (nodes, edges) in place on
// copies; shared by build and expand.
function merge(state: FlowState, anchorId: string, n: Neighbors, ghost = false): FlowState {
  const nodes = [...state.nodes]
  const edges = [...state.edges]
  const anchor = nodes.find((x) => x.id === anchorId)
  if (!anchor) return state
  const anchorRect = nodeRect(anchor)
  const boxSize = ghost ? { w: GHOST_W, h: GHOST_H } : { w: NODE_W, h: NODE_H }

  for (const d of directions(n)) {
    const fresh = d.rows.filter((r) => !nodes.some((x) => x.id === r.address))
    // place only fresh nodes; edges are added for every row
    if (fresh.length > 0) {
      const occupied = nodes.map(nodeRect)
      let rects: Rect[]
      if (d.dir === 'up') {
        // facilitators stack in a row-column above the anchor, offset left so
        // they do not collide with payee columns
        const x = anchorRect.x + 40
        rects = placeColumn(occupied, x, anchorRect.y - COL_GAP - boxSize.h, fresh.map(() => boxSize))
      } else {
        const x = columnX(anchorRect, d.dir, boxSize.w)
        rects = placeColumn(occupied, x, anchorRect.y + anchorRect.h / 2, fresh.map(() => boxSize))
      }
      fresh.forEach((r, i) => nodes.push(makeNode(r, ghost ? 'ghost' : d.roleClass, rects[i], ghost)))
    }
    for (const r of d.rows) {
      const [source, target] = d.toAnchor ? [r.address, anchorId] : [anchorId, r.address]
      const id = d.settles ? `${source}=>${target}` : `${source}->${target}`
      if (!edges.some((e) => e.id === id)) {
        edges.push(makeEdge(source, target, r, { settles: d.settles, ghost }))
      }
    }
  }
  return { nodes, edges }
}

export function buildEntityGraph(
  subject: { address: string; label?: string; roles: string[] },
  kpis: { payments: string; volume: string; counterparties: string },
  neighbors: Neighbors,
  ghosts: NeighborRow[],
): FlowState {
  const subjectNode: Node<EntityNodeData> = {
    id: subject.address,
    type: 'entity',
    position: { x: 0, y: 0 },
    data: {
      address: subject.address,
      label: subject.label,
      roleClass: 'subject',
      chips: subject.roles,
      kpis,
      expandable: false,
    },
  }
  let state: FlowState = { nodes: [subjectNode], edges: [] }
  state = merge(state, subject.address, neighbors)
  if (ghosts.length > 0) {
    // ghosts always render as payees-direction stubs under the subject's right lane
    state = merge(state, subject.address, { address: subject.address, lens: 'all', payees: { total: ghosts.length, rows: ghosts } }, true)
  }
  // primary = single highest-share flow edge touching the subject
  let best: Edge | undefined
  for (const e of state.edges) {
    const d = e.data as { share?: number; ghost?: boolean } | undefined
    if (d?.share == null || d.ghost) continue
    if (e.source !== subject.address && e.target !== subject.address) continue
    const bestShare = (best?.data as { share?: number } | undefined)?.share ?? -1
    if (d.share > bestShare) best = e
  }
  if (best) (best.data as { primary?: boolean }).primary = true
  return state
}

export function expandNode(state: FlowState, parentAddr: string, neighbors: Neighbors): FlowState {
  return merge(state, parentAddr, neighbors)
}

export function ghostRows(knownRows: NeighborRow[], allRows: NeighborRow[], cap: number): NeighborRow[] {
  const known = new Set(knownRows.map((r) => r.address))
  return allRows.filter((r) => !known.has(r.address)).slice(0, cap)
}
