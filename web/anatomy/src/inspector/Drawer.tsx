import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, type Lens } from '../lib/api'
import { dayLabel, short } from '../lib/format'
import { primaryRole } from '../lib/roles'
import type { GraphNode } from '../lib/schemas'
import { OverviewTab } from './OverviewTab'
import { ActivityTab } from './ActivityTab'
import { FingerprintTab } from './FingerprintTab'
import { CounterpartiesTab } from './CounterpartiesTab'
import { PaymentsTab } from './PaymentsTab'
import { TxDetail } from './TxDetail'

const TABS = ['Overview', 'Activity', 'Fingerprint', 'Counterparties', 'Payments'] as const

type Props = {
  chain: string
  address: string
  lens: Lens
  onNavigate: (to: string) => void
  txNode?: GraphNode // when set (tx route, tx node selected), a Tx tab leads
}

export function Drawer({ chain, address, lens, onNavigate, txNode }: Props) {
  const tabs = txNode ? (['Tx', ...TABS] as const) : TABS
  const [tab, setTab] = useState<string>(tabs[0])
  const entity = useQuery({
    queryKey: ['entity', chain, address],
    queryFn: () => api.entity(chain, address),
    enabled: !txNode || tab !== 'Tx',
  })
  const e = entity.data
  const role = e ? primaryRole(e.roles) : undefined
  const sum = e && role ? e.summaries[role]?.[lens] : undefined
  return (
    <aside className="drawer">
      <div className="drawer-inner">
        <div className="d-head">
          <div className="d-title-row">
            <span className="d-addr">{txNode ? short(txNode.label) : e?.label ?? short(address)}</span>
            <span className="badge" data-testid="d-badge">{txNode ? 'transaction' : e?.labelSource ?? 'Unidentified'}</span>
            {role && <span className="chip cyan" style={{ marginLeft: 'auto' }}>{role}</span>}
          </div>
          {sum && (
            <div className="d-meta">
              <span>First seen <b>{dayLabel(sum.firstDay ?? '')}</b></span>
              <span>Last seen <b>{dayLabel(sum.lastDay ?? '')}</b></span>
              <span>Active <b>{sum.activeDays}</b> days</span>
            </div>
          )}
        </div>
        <div className="tabs">
          {tabs.map((t) => (
            <button key={t} className={`tab ${t === tab ? 'active' : ''}`} onClick={() => setTab(t)}>{t}</button>
          ))}
        </div>
        {tab === 'Tx' && txNode && <TxDetail node={txNode} />}
        {tab === 'Overview' && (
          entity.isLoading ? <div className="skeleton">loading identity…</div> :
          entity.isError ? <div className="banner-err">{String(entity.error)}</div> :
          e ? <OverviewTab entity={e} lens={lens} /> : null
        )}
        {tab === 'Activity' && <ActivityTab chain={chain} address={address} lens={lens} />}
        {tab === 'Fingerprint' && <FingerprintTab chain={chain} address={address} lens={lens} />}
        {tab === 'Counterparties' && <CounterpartiesTab chain={chain} address={address} lens={lens} onNavigate={onNavigate} defaultRole={role ?? 'payer'} />}
        {tab === 'Payments' && <PaymentsTab chain={chain} address={address} lens={lens} onNavigate={onNavigate} defaultRole={role ?? 'payer'} />}
        <div className="drawer-foot" style={{ marginTop: 'auto', paddingTop: 10 }}>
          <span>Lens: <b>{lens === 'known' ? 'verified x402' : 'all EIP-3009'}</b></span>
        </div>
      </div>
    </aside>
  )
}
