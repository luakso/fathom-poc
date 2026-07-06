// The timeline endpoint returns sparse day rows; the client densifies so bar
// charts show real gaps (spec §5). UTC day arithmetic, no wall-clock use.

export type DayPoint = { day: string; txnCount: number; volumeUsdc: string }

export function densify(points: DayPoint[]): DayPoint[] {
  if (points.length < 2) return [...points]
  const sorted = [...points].sort((a, b) => (a.day < b.day ? -1 : 1))
  const byDay = new Map(sorted.map((p) => [p.day, p]))
  const out: DayPoint[] = []
  const first = sorted[0]
  const last = sorted[sorted.length - 1]
  if (!first || !last) return [...points]
  const end = Date.parse(`${last.day}T00:00:00Z`)
  for (let t = Date.parse(`${first.day}T00:00:00Z`); t <= end; t += 86_400_000) {
    const day = new Date(t).toISOString().slice(0, 10)
    out.push(byDay.get(day) ?? { day, txnCount: 0, volumeUsdc: '0' })
  }
  return out
}
