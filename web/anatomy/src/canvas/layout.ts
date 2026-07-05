// Deterministic incremental lane layout (spec §6): payers left, payees right,
// facilitators above. New nodes stack in a column and get pushed down past
// any occupied rect. No elkjs in v1; the no-overlap invariant is tested.

export const NODE_W = 300
export const NODE_H = 150
export const GHOST_W = 230
export const GHOST_H = 80
export const COL_GAP = 240
export const ROW_GAP = 36

export type Rect = { x: number; y: number; w: number; h: number }
export type Dir = 'left' | 'right' | 'up'

export function overlaps(a: Rect, b: Rect): boolean {
  return a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
}

export function columnX(anchor: Rect, dir: Dir, w: number): number {
  if (dir === 'right') return anchor.x + anchor.w + COL_GAP
  if (dir === 'left') return anchor.x - COL_GAP - w
  return anchor.x
}

// Column collision only matters against rects whose x-range intersects the
// new column; pre-filtering keeps the scan linear in column occupancy.
function collider(occupied: Rect[], probe: Rect): Rect | undefined {
  return occupied.find((o) => overlaps(probe, o))
}

export function placeColumn(
  occupied: Rect[],
  x: number,
  centerY: number,
  sizes: { w: number; h: number }[],
): Rect[] {
  const totalH = sizes.reduce((s, b) => s + b.h, 0) + ROW_GAP * Math.max(0, sizes.length - 1)
  let y = centerY - totalH / 2
  const placed: Rect[] = []
  for (const size of sizes) {
    let rect: Rect = { x, y, w: size.w, h: size.h }
    for (;;) {
      const hit = collider([...occupied, ...placed], rect)
      if (!hit) break
      rect = { ...rect, y: hit.y + hit.h + ROW_GAP }
    }
    placed.push(rect)
    y = rect.y + rect.h + ROW_GAP
  }
  return placed
}
