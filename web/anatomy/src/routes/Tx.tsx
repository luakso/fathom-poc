import { useMemo } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, ApiError, type Lens } from '../lib/api'
import { txGraphToFlow } from '../canvas/txgraph'
import { Canvas } from '../canvas/Canvas'
import { Drawer } from '../inspector/Drawer'

const CHAIN = 'base'

export function Tx() {
  const { hash = '' } = useParams()
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const lens = (params.get('lens') === 'all' ? 'all' : 'known') as Lens
  const q = useQuery({ queryKey: ['tx', CHAIN, hash], queryFn: () => api.tx(CHAIN, hash), retry: false })
  const flow = useMemo(() => (q.data ? txGraphToFlow(q.data) : { nodes: [], edges: [] }), [q.data])
  const txNodeId = q.data?.nodes.find((n) => n.kind === 'transaction')?.id
  const sel = params.get('sel') ?? txNodeId
  const selNode = q.data?.nodes.find((n) => n.id === sel)

  if (q.isError && q.error instanceof ApiError && q.error.status === 404) {
    return <div className="empty-state" style={{ flex: 1, alignSelf: 'center' }}>unknown transaction</div>
  }
  if (q.isError) return <div className="banner-err" style={{ margin: 20 }}>{String(q.error)}</div>
  if (q.isLoading) return <div className="skeleton" style={{ margin: 20 }}>loading dossier…</div>

  const selAddr = selNode?.kind === 'address' ? (selNode.fields.address ?? selNode.label) : ''
  return (
    <div className="main-split">
      {q.data?.truncated && (
        <div className="banner-err" style={{ position: 'absolute', top: 70, left: 20, zIndex: 40 }}>
          showing first 128 events (truncated)
        </div>
      )}
      <Canvas
        nodes={flow.nodes} edges={flow.edges} selectedId={sel}
        onSelect={(id) => { const p = new URLSearchParams(params); p.set('sel', id); setParams(p) }}
        onFocus={(a) => navigate(`/base/address/${a}${lens === 'all' ? '?lens=all' : ''}`)}
      />
      {/* key remounts the Drawer whenever the selected node changes so its tab
          state resets - otherwise selecting an address after the tx node
          leaves the drawer stuck on the now-absent 'Tx' tab (and vice versa
          selecting the tx node after an address carries over that address's
          stale tab/paging state). */}
      <Drawer
        key={`${sel ?? ''}:${lens}`}
        chain={CHAIN}
        address={selAddr || (q.data?.nodes.find((n) => n.kind === 'address')?.fields.address ?? '')}
        lens={lens}
        onNavigate={(to) => navigate(to)}
        txNode={selNode?.kind === 'transaction' ? selNode : undefined}
      />
    </div>
  )
}
