import React from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { AppRoutes, newQueryClient } from './App'
import './theme.css'

const rootEl = document.getElementById('root')
if (!rootEl) throw new Error('root element (#root) not found - cannot mount the app')

createRoot(rootEl).render(
  <React.StrictMode>
    <BrowserRouter>
      <QueryClientProvider client={newQueryClient()}>
        <AppRoutes />
      </QueryClientProvider>
    </BrowserRouter>
  </React.StrictMode>,
)
