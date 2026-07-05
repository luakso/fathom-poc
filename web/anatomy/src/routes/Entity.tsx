import { useEffect, useState, useCallback } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError, type Lens } from '../lib/api'
import { usd, groupDigits } from '../lib/format'
import { primaryRole } from '../lib/roles'
import { buildEntityGraph, expandNode, ghostRows, type FlowState } from '../canvas/graph'
import { Canvas } from '../canvas/Canvas'
import { Drawer } from '../inspector/Drawer'
import type { Neighbors, NeighborRow } from '../lib/schemas'

const CHAIN = 'base'

function directionRows(n: Neighbors | undefined): NeighborRow[] {
  return [...(n?.payers?.rows ?? []), ...(n?.payees?.rows ?? [])]
}

export function Entity() {
  const { addr = '' } = useParams()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const lens = (params.get('lens') === 'all' ? 'all' : 'known') as Lens
  const sel = params.get('sel') ?? addr
  const [flow, setFlow] = useState<FlowState>({ nodes: [], edges: [] })
  const [expandErr, setExpandErr] = useState('')

  const entity = useQuery({ queryKey: ['entity', CHAIN, addr], queryFn: () => api.entity(CHAIN, addr), retry: false })
  const neighbors = useQuery({ queryKey: ['neighbors', CHAIN, addr, lens], queryFn: () => api.neighbors(CHAIN, addr, lens) })
  const allNeighbors = useQuery({
    queryKey: ['neighbors', CHAIN, addr, 'all'],
    queryFn: () => api.neighbors(CHAIN, addr, 'all'),
    enabled: lens === 'known',
  })

  useEffect(() => {
    const e = entity.data
    const n = neighbors.data
    if (!e || !n) return
    const role = primaryRole(e.roles)
    const sum = e.summaries[role]?.[lens]
    const kpis = {
      payments: groupDigits(sum?.txnCount ?? 0),
      volume: usd(sum?.volumeUsdc ?? '0'),
      counterparties: groupDigits(sum?.distinctCounterparties ?? 0),
    }
    const ghosts = lens === 'known'
      ? ghostRows(directionRows(n), directionRows(allNeighbors.data), 4)
      : []
    setFlow(buildEntityGraph({ address: addr, label: e.label, roles: e.roles }, kpis, n, ghosts))
  }, [addr, lens, entity.data, neighbors.data, allNeighbors.data])

  const onExpand = useCallback(async (a: string) => {
    try {
      const n = await api.neighbors(CHAIN, a, lens)
      setFlow((f) => expandNode(f, a, n))
    } catch (err) {
      setExpandErr(err instanceof Error ? err.message : 'expand failed')
    }
  }, [lens])

  const onSelect = useCallback((id: string) => {
    const p = new URLSearchParams(params)
    p.set('sel', id)
    setParams(p, { replace: false })
  }, [params, setParams])

  const onFocus = useCallback((a: string) => {
    navigate(`/base/address/${a}${lens === 'all' ? '?lens=all' : ''}`)
  }, [navigate, lens])

  if (entity.isError && entity.error instanceof ApiError && entity.error.status === 404) {
    return <div className="empty-state" style={{ flex: 1, alignSelf: 'center' }}>unknown address - nothing indexed under either lens</div>
  }
  if (entity.isError) {
    return <div className="banner-err" style={{ margin: 20 }}>{String(entity.error)}</div>
  }

  // zero-activity escape hatch: nothing under this lens, something under the other
  const e = entity.data
  if (e) {
    const role = primaryRole(e.roles)
    const here = e.summaries[role]?.[lens]?.txnCount ?? 0
    const other: Lens = lens === 'known' ? 'all' : 'known'
    const there = e.summaries[role]?.[other]?.txnCount ?? 0
    if (here === 0 && there > 0) {
      return (
        <div className="empty-state" style={{ flex: 1, alignSelf: 'center' }}>
          nothing under this lens - switch lens: {groupDigits(there)} payments under {other === 'all' ? 'all EIP-3009' : 'verified x402'}
        </div>
      )
    }
  }

  return (
    <div className="main-split">
      {expandErr && (
        <div className="banner-err" style={{ position: 'absolute', top: 70, left: 20, zIndex: 40, cursor: 'pointer' }}
          onClick={() => setExpandErr('')}>
          {expandErr} (click to dismiss)
        </div>
      )}
      <Canvas nodes={flow.nodes} edges={flow.edges} selectedId={sel} onSelect={onSelect} onExpand={onExpand} onFocus={onFocus} />
      {/* key remounts the Drawer on selection/lens change so its internal tab
          and per-tab paging state (PaymentsTab cursor, CounterpartiesTab
          offset, etc.) resets instead of carrying over from the previous
          bound address. */}
      <Drawer key={`${sel}:${lens}`} chain={CHAIN} address={sel} lens={lens} onNavigate={(to) => navigate(to)} />
    </div>
  )
}
