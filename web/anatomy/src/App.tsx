import { Routes, Route } from 'react-router-dom'
import { QueryClient } from '@tanstack/react-query'

export function newQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: 1, staleTime: 60_000, refetchOnWindowFocus: false } },
  })
}

function Placeholder({ name }: { name: string }) {
  return <div className="empty-state">{name}</div>
}

export function AppRoutes() {
  return (
    <div className="layout">
      <Routes>
        <Route path="/" element={<Placeholder name="home" />} />
        <Route path="/base/address/:addr" element={<Placeholder name="entity" />} />
        <Route path="/base/tx/:hash" element={<Placeholder name="tx" />} />
        <Route path="*" element={<Placeholder name="not found" />} />
      </Routes>
    </div>
  )
}
