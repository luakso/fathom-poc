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
import type { RoleClass } from '../lib/roles'

// Data payload for entity/ghost cards. Explicit optional fields (no index
// signature) so the node data stays fully typed; @xyflow only requires that a
// node's data be assignable to Record<string, unknown>, which this object type
// satisfies. onExpand/onFocus are injected by <Canvas> at render time.
export type EntityNodeData = {
  address: string
  label?: string
  roleClass: RoleClass
  chips: string[]
  kpis?: { payments: string; volume: string; counterparties: string }
  detail?: string
  expandable: boolean
  onExpand?: (addr: string) => void
  onFocus?: (addr: string) => void
}

// Data payloads for the transaction dossier's tx/event cards (built in txgraph).
export type TxNodeData = { label: string; fields: Record<string, string> }
export type EventNodeData = { label: string; fields: Record<string, string> }

// Edge payload shared by both the entity canvas and the tx dossier.
export type FlowEdgeData = {
  share?: number
  label?: string
  primary?: boolean
  verb?: boolean
  ghost?: boolean
}

export type EntityFlowNode = Node<EntityNodeData>
export type TxFlowNode = Node<TxNodeData>
export type EventFlowNode = Node<EventNodeData>
export type FlowNode = EntityFlowNode | TxFlowNode | EventFlowNode
export type FlowEdge = Edge<FlowEdgeData>

export type FlowState = { nodes: FlowNode[]; edges: FlowEdge[] }

const SUBJECT_H = NODE_H + 40

function nodeRect(n: FlowNode): Rect {
  const ghost = n.type === 'ghost'
  const subject = 'roleClass' in n.data && n.data.roleClass === 'subject'
  return {
    x: n.position.x,
    y: n.position.y,
    w: ghost ? GHOST_W : NODE_W,
    h: ghost ? GHOST_H : subject ? SUBJECT_H : NODE_H,
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
): EntityFlowNode {
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

function makeEdge(source: string, target: string, r: NeighborRow, opts: { settles?: boolean; ghost?: boolean }): FlowEdge {
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
      fresh.forEach((r, i) => {
        const rect = rects[i]
        if (rect) nodes.push(makeNode(r, ghost ? 'ghost' : d.roleClass, rect, ghost))
      })
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
  const subjectNode: EntityFlowNode = {
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
  // primary = single highest-share flow edge touching the subject. Computed as
  // an id, then applied immutably so no rendered edge object is mutated.
  let bestId: string | undefined
  let bestShare = -1
  for (const e of state.edges) {
    const d = e.data
    if (d?.share == null || d.ghost) continue
    if (e.source !== subject.address && e.target !== subject.address) continue
    if (d.share > bestShare) {
      bestShare = d.share
      bestId = e.id
    }
  }
  if (bestId === undefined) return state
  const edges = state.edges.map((e) =>
    e.id === bestId ? { ...e, data: { ...e.data, primary: true } } : e,
  )
  return { ...state, edges }
}

export function expandNode(state: FlowState, parentAddr: string, neighbors: Neighbors): FlowState {
  return merge(state, parentAddr, neighbors)
}

export function ghostRows(knownRows: NeighborRow[], allRows: NeighborRow[], cap: number): NeighborRow[] {
  const known = new Set(knownRows.map((r) => r.address))
  return allRows.filter((r) => !known.has(r.address)).slice(0, cap)
}
