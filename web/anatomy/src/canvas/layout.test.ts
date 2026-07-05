import { describe, it, expect } from 'vitest'
import { overlaps, columnX, placeColumn, NODE_W, NODE_H, COL_GAP, ROW_GAP, Rect } from './layout'

const box = { w: NODE_W, h: NODE_H }

function assertNoOverlaps(rects: Rect[]) {
  for (let i = 0; i < rects.length; i++) {
    for (let j = i + 1; j < rects.length; j++) {
      expect(overlaps(rects[i], rects[j]), `rect ${i} overlaps rect ${j}`).toBe(false)
    }
  }
}

describe('overlaps', () => {
  it('detects intersection and respects edges', () => {
    const a = { x: 0, y: 0, w: 100, h: 100 }
    expect(overlaps(a, { x: 50, y: 50, w: 100, h: 100 })).toBe(true)
    expect(overlaps(a, { x: 100, y: 0, w: 100, h: 100 })).toBe(false)
    expect(overlaps(a, { x: 0, y: 100, w: 100, h: 100 })).toBe(false)
  })
})

describe('columnX', () => {
  const anchor = { x: 0, y: 0, w: NODE_W, h: NODE_H }
  it('right column starts one gap past the anchor right edge', () => {
    expect(columnX(anchor, 'right', NODE_W)).toBe(NODE_W + COL_GAP)
  })
  it('left column ends one gap before the anchor left edge', () => {
    expect(columnX(anchor, 'left', NODE_W)).toBe(-(COL_GAP + NODE_W))
  })
  it('up keeps the anchor x', () => {
    expect(columnX(anchor, 'up', NODE_W)).toBe(0)
  })
})

describe('placeColumn', () => {
  it('centers a stack on centerY', () => {
    const rects = placeColumn([], 600, 0, [box, box])
    expect(rects).toHaveLength(2)
    const top = rects[0].y
    const bottom = rects[1].y + rects[1].h
    expect(Math.abs(top + bottom)).toBeLessThanOrEqual(1) // symmetric around 0
    assertNoOverlaps(rects)
  })
  it('never overlaps occupied rects', () => {
    const occupied = [{ x: 600, y: -80, w: NODE_W, h: NODE_H }]
    const rects = placeColumn(occupied, 600, 0, [box, box, box])
    assertNoOverlaps([...occupied, ...rects])
  })
  it('is deterministic', () => {
    const a = placeColumn([], 600, 100, [box, box, box])
    expect(placeColumn([], 600, 100, [box, box, box])).toEqual(a)
  })
  it('holds the invariant across a simulated expansion cascade', () => {
    // subject at origin; 8 payees right; then one payee expands 8 more further right,
    // overlapping the first column's y-range - everything must still be disjoint.
    const all: Rect[] = [{ x: 0, y: 0, w: NODE_W, h: NODE_H }]
    const c1 = placeColumn(all, NODE_W + COL_GAP, NODE_H / 2, Array(8).fill(box))
    all.push(...c1)
    const anchor = c1[3]
    const c2 = placeColumn(all, columnX(anchor, 'right', NODE_W), anchor.y + anchor.h / 2, Array(8).fill(box))
    all.push(...c2)
    const c3 = placeColumn(all, columnX(anchor, 'left', NODE_W), anchor.y, Array(4).fill(box))
    all.push(...c3)
    assertNoOverlaps(all)
    expect(all).toHaveLength(21)
  })
  it('ROW_GAP separates stacked cards', () => {
    const [a, b] = placeColumn([], 0, 0, [box, box])
    expect(b.y - (a.y + a.h)).toBe(ROW_GAP)
  })
})
