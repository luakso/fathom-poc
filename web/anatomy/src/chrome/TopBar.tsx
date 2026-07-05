import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { detectQuery } from '../lib/detect'
import { dayLabel } from '../lib/format'

export function TopBar() {
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const lens = (params.get('lens') === 'all' ? 'all' : 'known') as Lens
  const [q, setQ] = useState('')
  const [err, setErr] = useState('')
  const meta = useQuery({ queryKey: ['meta'], queryFn: api.meta })

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const d = detectQuery(q)
    if (d.kind === 'invalid') {
      setErr('not an address or tx hash')
      return
    }
    setErr('')
    setQ('')
    const suffix = lens === 'all' ? '?lens=all' : ''
    navigate(d.kind === 'address' ? `/base/address/${d.value}${suffix}` : `/base/tx/${d.value}${suffix}`)
  }

  function setLens(next: Lens) {
    const p = new URLSearchParams(params)
    if (next === 'all') p.set('lens', 'all')
    else p.delete('lens')
    setParams(p)
  }

  return (
    <header className="chrome">
      <div className="brand">
        <span className="wordmark">Anat<em>o</em>my</span>
        <span className="env-tag">x402 · base</span>
      </div>
      <form className="search" onSubmit={submit}>
        <input
          value={q}
          placeholder="address or tx hash"
          onChange={(e) => { setQ(e.target.value); setErr('') }}
        />
        {err && <span className="search-err">{err}</span>}
      </form>
      <div className="lens">
        <button type="button" className={lens === 'known' ? 'active' : ''} onClick={() => setLens('known')}>
          <span className="dot" />Verified x402
        </button>
        <button type="button" className={lens === 'all' ? 'active' : ''} onClick={() => setLens('all')}>
          All EIP-3009
        </button>
      </div>
      <div className="asof">
        {meta.data ? <>Data as of <b>{dayLabel(meta.data.dataMaxDay)}</b></> : 'Data as of —'}
      </div>
    </header>
  )
}
