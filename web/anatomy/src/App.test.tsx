// @vitest-environment jsdom
import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { AppRoutes, newQueryClient } from './App'

describe('App boot', () => {
  it('renders the home route without crashing', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <QueryClientProvider client={newQueryClient()}>
          <AppRoutes />
        </QueryClientProvider>
      </MemoryRouter>,
    )
    expect(screen.getByText('home')).toBeTruthy()
  })
})
