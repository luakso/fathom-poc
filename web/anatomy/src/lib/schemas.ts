import { z } from 'zod'

// The three economic roles are a closed set across every API surface (entity
// roles, graph-node roles, timeline/fingerprint role keys). Enumerated here so
// an unexpected role fails loudly at the boundary rather than flowing through
// as an opaque string.
export const RoleSchema = z.enum(['payer', 'payee', 'facilitator'])
export type Role = z.infer<typeof RoleSchema>

export const LensTotalsSchema = z.object({ txnCount: z.number(), volumeUsdc: z.string() })
export const MetaSchema = z.object({
  dataMaxDay: z.string(),
  builtAt: z.string(),
  methodologyVersion: z.number(),
  totals: z.record(z.string(), LensTotalsSchema),
})
export type Meta = z.infer<typeof MetaSchema>

export const LensSummarySchema = z.object({
  txnCount: z.number(),
  volumeUsdc: z.string(),
  firstDay: z.string().optional(),
  lastDay: z.string().optional(),
  activeDays: z.number(),
  distinctCounterparties: z.number(),
})
export type LensSummary = z.infer<typeof LensSummarySchema>

export const IdentitySignalSchema = z.object({
  source: z.string(),
  kind: z.string(),
  value: z.string(),
  url: z.string().optional(),
  fetchedAt: z.string().optional(),
})
export type IdentitySignal = z.infer<typeof IdentitySignalSchema>

export const EntitySchema = z.object({
  chain: z.string(),
  address: z.string(),
  label: z.string().optional(),
  labelSource: z.string().optional(),
  roles: z.array(RoleSchema),
  signals: z.array(IdentitySignalSchema).optional(),
  summaries: z.record(z.string(), z.record(z.string(), LensSummarySchema)),
})
export type Entity = z.infer<typeof EntitySchema>

export const NeighborRowSchema = z.object({
  address: z.string(),
  label: z.string().optional(),
  txnCount: z.number(),
  volumeUsdc: z.string(),
  share: z.string(),
  firstSeen: z.string(),
  lastSeen: z.string(),
})
export type NeighborRow = z.infer<typeof NeighborRowSchema>

export const NeighborListSchema = z.object({ total: z.number(), rows: z.array(NeighborRowSchema) })
export type NeighborList = z.infer<typeof NeighborListSchema>

export const NeighborsSchema = z.object({
  address: z.string(),
  lens: z.string(),
  payees: NeighborListSchema.optional(),
  payers: NeighborListSchema.optional(),
  facilitators: NeighborListSchema.optional(),
  settledPayers: NeighborListSchema.optional(),
  settledPayees: NeighborListSchema.optional(),
})
export type Neighbors = z.infer<typeof NeighborsSchema>

export const DayPointSchema = z.object({ day: z.string(), txnCount: z.number(), volumeUsdc: z.string() })
export const TimelineSchema = z.object({
  address: z.string(),
  lens: z.string(),
  roles: z.record(z.string(), z.array(DayPointSchema)),
})
export type Timeline = z.infer<typeof TimelineSchema>

export const PricePointSchema = z.object({ amountUsdc: z.string(), txnCount: z.number() })
export const RoleFingerprintSchema = z.object({
  activeDays: z.number(),
  spanDays: z.number(),
  medianTxnsPerDay: z.number(),
  topDayShare: z.string(),
  pricePoints: z.array(PricePointSchema),
  totalDistinctAmounts: z.number().nullable(),
  top1Share: z.string(),
  top3Share: z.string(),
})
export const FingerprintSchema = z.object({
  address: z.string(),
  lens: z.string(),
  roles: z.record(z.string(), RoleFingerprintSchema),
})
export type Fingerprint = z.infer<typeof FingerprintSchema>

export const CounterpartyPageSchema = z.object({
  address: z.string(),
  role: z.string(),
  lens: z.string(),
  total: z.number(),
  rows: z.array(NeighborRowSchema),
})
export type CounterpartyPage = z.infer<typeof CounterpartyPageSchema>

export const PaymentRowSchema = z.object({
  txHash: z.string(),
  logIndex: z.number(),
  blockNumber: z.number(),
  blockTimestamp: z.string(),
  payer: z.string(),
  payee: z.string(),
  facilitator: z.string(),
  amountUsdc: z.string(),
  facilitatorKnown: z.boolean(),
})
export type PaymentRow = z.infer<typeof PaymentRowSchema>

export const PaymentPageSchema = z.object({
  address: z.string(),
  role: z.string(),
  lens: z.string(),
  rows: z.array(PaymentRowSchema),
  next: z.string().optional(),
})
export type PaymentPage = z.infer<typeof PaymentPageSchema>

export const LeaderboardRowSchema = z.object({
  rank: z.number(),
  address: z.string(),
  label: z.string().optional(),
  txnCount: z.number(),
  volumeUsdc: z.string(),
  distinctCounterparties: z.number(),
  firstSeen: z.string(),
  lastSeen: z.string(),
})
export type LeaderboardRow = z.infer<typeof LeaderboardRowSchema>

export const LeaderboardSchema = z.object({
  role: z.string(),
  window: z.string(),
  lens: z.string(),
  sort: z.string(),
  rows: z.array(LeaderboardRowSchema),
})
export type Leaderboard = z.infer<typeof LeaderboardSchema>

export const ProviderRefSchema = z.object({ kind: z.string(), available: z.boolean() })
export const GraphNodeSchema = z.object({
  id: z.string(),
  kind: z.enum(['transaction', 'event', 'address']),
  label: z.string(),
  roles: z.array(RoleSchema).optional(),
  fields: z.record(z.string(), z.string()),
  providers: z.array(ProviderRefSchema).optional(),
})
export type GraphNode = z.infer<typeof GraphNodeSchema>
export const GraphEdgeSchema = z.object({
  id: z.string(),
  source: z.string(),
  target: z.string(),
  label: z.string().optional(),
  kind: z.string(),
})
export type GraphEdge = z.infer<typeof GraphEdgeSchema>
export const GraphSchema = z.object({
  chain: z.string(),
  txHash: z.string(),
  nodes: z.array(GraphNodeSchema),
  edges: z.array(GraphEdgeSchema),
  truncated: z.boolean().optional(),
})
export type Graph = z.infer<typeof GraphSchema>
