import { Routes, Route } from 'react-router-dom'
import { QueryClient } from '@tanstack/react-query'
import { TopBar } from './chrome/TopBar'
import { Home } from './routes/Home'
import { Entity } from './routes/Entity'
import { Tx } from './routes/Tx'

export function newQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: 1, staleTime: 60_000, refetchOnWindowFocus: false } },
  })
}

export function AppRoutes() {
  return (
    <div className="layout">
      <TopBar />
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/base/address/:addr" element={<Entity />} />
        <Route path="/base/tx/:hash" element={<Tx />} />
        <Route path="*" element={<div className="empty-state">not found</div>} />
      </Routes>
    </div>
  )
}
