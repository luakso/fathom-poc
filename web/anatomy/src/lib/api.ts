// Typed API client. Every response is zod-parsed at the boundary (spec §6):
// a typo'd field fails loudly here, never deep in a component.
import { z } from 'zod'
import {
  MetaSchema, EntitySchema, NeighborsSchema, TimelineSchema, FingerprintSchema,
  CounterpartyPageSchema, PaymentPageSchema, LeaderboardSchema, GraphSchema,
} from './schemas'
import type {
  Meta, Entity, Neighbors, Timeline, Fingerprint,
  CounterpartyPage, PaymentPage, Leaderboard, Graph,
} from './schemas'

export type Lens = 'known' | 'all'

// Shape of the Go API's error envelope. Parsed defensively: a non-conforming
// error body falls back to the status line rather than leaking `[object Object]`.
const ApiErrorSchema = z.object({ error: z.string() }).partial()

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message)
    this.name = 'ApiError'
  }
}

async function getJSON<T>(url: string, schema: z.ZodType<T>): Promise<T> {
  const res = await fetch(url)
  const body: unknown = await res.json().catch(() => null)
  if (!res.ok) {
    const parsed = ApiErrorSchema.safeParse(body)
    const msg = parsed.success && parsed.data.error ? parsed.data.error : `request failed: ${res.status}`
    throw new ApiError(msg, res.status)
  }
  return schema.parse(body)
}

function qs(params: Record<string, string | number | undefined>): string {
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') p.set(k, String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  meta: (): Promise<Meta> => getJSON('/api/meta', MetaSchema),
  entity: (chain: string, addr: string): Promise<Entity> =>
    getJSON(`/api/${chain}/entity/${addr}`, EntitySchema),
  neighbors: (chain: string, addr: string, lens: Lens, limit?: number): Promise<Neighbors> =>
    getJSON(`/api/${chain}/entity/${addr}/neighbors${qs({ lens, limit })}`, NeighborsSchema),
  timeline: (chain: string, addr: string, lens: Lens): Promise<Timeline> =>
    getJSON(`/api/${chain}/entity/${addr}/timeline${qs({ lens })}`, TimelineSchema),
  fingerprint: (chain: string, addr: string, lens: Lens): Promise<Fingerprint> =>
    getJSON(`/api/${chain}/entity/${addr}/fingerprint${qs({ lens })}`, FingerprintSchema),
  counterparties: (
    chain: string, addr: string,
    q: { role: string; lens: Lens; sort?: string; limit?: number; offset?: number },
  ): Promise<CounterpartyPage> =>
    getJSON(`/api/${chain}/entity/${addr}/counterparties${qs(q)}`, CounterpartyPageSchema),
  payments: (
    chain: string, addr: string,
    q: { role: string; lens: Lens; limit?: number; before?: string },
  ): Promise<PaymentPage> =>
    getJSON(`/api/${chain}/entity/${addr}/payments${qs(q)}`, PaymentPageSchema),
  leaderboard: (
    chain: string,
    q: { role: string; window: string; lens: Lens; sort: string; limit?: number },
  ): Promise<Leaderboard> =>
    getJSON(`/api/${chain}/leaderboard${qs(q)}`, LeaderboardSchema),
  tx: (chain: string, hash: string): Promise<Graph> =>
    getJSON(`/api/${chain}/tx/${hash.toLowerCase()}`, GraphSchema),
}
