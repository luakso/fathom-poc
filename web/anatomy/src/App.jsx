import { useCallback, useMemo, useState } from 'react'
import { ReactFlow, Background, Controls } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { fetchTx, fetchStats } from './api.js'
import { graphToFlow, applyStats } from './adapter.js'
import TransactionNode from './nodes/TransactionNode.jsx'
import EventNode from './nodes/EventNode.jsx'
import AddressNode from './nodes/AddressNode.jsx'

export default function App() {
  const [chain, setChain] = useState('base')
  const [hash, setHash] = useState('')
  const [flow, setFlow] = useState({ nodes: [], edges: [] })
  const [err, setErr] = useState('')

  const trace = useCallback(async () => {
    setErr('')
    try {
      const graph = await fetchTx(chain, hash.trim())
      setFlow(graphToFlow(graph))
    } catch (e) {
      setErr(e.message)
      setFlow({ nodes: [], edges: [] })
    }
  }, [chain, hash])

  const expandStats = useCallback(async (addrId) => {
    const addr = addrId.replace(/^addr:/, '')
    try {
      const stats = await fetchStats(chain, addr)
      setFlow((f) => applyStats(f, addrId, stats))
    } catch (e) {
      setErr(e.message)
    }
  }, [chain])

  const decoratedNodes = useMemo(
    () => flow.nodes.map((n) =>
      n.type === 'address'
        ? { ...n, data: { ...n.data, onExpandStats: expandStats } }
        : n,
    ),
    [flow.nodes, expandStats],
  )

  const nodeTypes = useMemo(
    () => ({ transaction: TransactionNode, event: EventNode, address: AddressNode }),
    [],
  )

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <div className="toolbar">
        <select value={chain} onChange={(e) => setChain(e.target.value)}>
          <option value="base">base</option>
          <option value="solana">solana</option>
        </select>
        <input
          value={hash}
          placeholder="paste a transaction hash…"
          onChange={(e) => setHash(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && trace()}
        />
        <button onClick={trace}>trace</button>
      </div>
      {err && <div className="banner-err">{err}</div>}
      <div style={{ flex: 1 }}>
        <ReactFlow nodes={decoratedNodes} edges={flow.edges} nodeTypes={nodeTypes} fitView>
          <Background />
          <Controls />
        </ReactFlow>
      </div>
    </div>
  )
}
