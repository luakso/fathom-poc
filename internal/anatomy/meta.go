package anatomy

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// metaTotals is the cached, immutable-after-publish per-lens totals for one
// built_at stamp. Once stored it is never mutated, so it is safe to hand the
// same map to concurrent readers.
type metaTotals struct {
	key    string // built_at the totals were computed for
	totals map[string]LensTotals
}

// PgMeta serves /api/meta from anatomy_meta plus lens totals aggregated from
// entity_day_v1 (payer role carries every payment exactly once). Totals are
// cached per built_at stamp: they only change when a rollup runs.
type PgMeta struct {
	pool   *pgxpool.Pool
	cached atomic.Pointer[metaTotals] // lock-free; swapped wholesale on a cache miss
}

// NewPgMeta constructs a PgMeta backed by the given pool.
func NewPgMeta(pool *pgxpool.Pool) *PgMeta { return &PgMeta{pool: pool} }

// Meta returns the latest rollup metadata. Returns ErrNotFound when no rollup
// has been run yet (anatomy_meta has no rows).
//
// The built_at stamp is read WITHOUT holding any lock; on a cache hit the
// totals come straight from an atomic pointer, so warm calls never serialize
// and never hold a lock across a DB round-trip. On a miss the totals are
// recomputed and the pointer is swapped wholesale — two concurrent misses may
// both compute (idempotent, last write wins), which is cheaper than serializing
// every caller behind one mutex.
func (p *PgMeta) Meta(ctx context.Context) (Meta, error) {
	var m Meta
	var builtAt string
	err := p.pool.QueryRow(ctx, `
		SELECT COALESCE(data_max_day::text, ''), built_at::text, methodology_version
		FROM anatomy_meta WHERE id = 1`).Scan(&m.DataMaxDay, &builtAt, &m.MethodologyVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Meta{}, ErrNotFound
	}
	if err != nil {
		return Meta{}, fmt.Errorf("read anatomy_meta: %w", err)
	}
	m.BuiltAt = builtAt

	if c := p.cached.Load(); c != nil && c.key == builtAt {
		m.Totals = c.totals
		return m, nil
	}
	totals, err := p.readTotals(ctx)
	if err != nil {
		return Meta{}, err
	}
	p.cached.Store(&metaTotals{key: builtAt, totals: totals})
	m.Totals = totals
	return m, nil
}

// readTotals computes per-lens aggregate counts from entity_day_v1.
// Volumes are summed in SQL (never in Go) to preserve decimal exactness.
// The UNION ALL avoids the grouping-sets NULL-collision trap for the two
// lens rows.
func (p *PgMeta) readTotals(ctx context.Context) (map[string]LensTotals, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT lens, sum(txn_count), sum(volume_usdc)::text FROM (
		    SELECT 'all' AS lens, txn_count, volume_usdc FROM entity_day_v1 WHERE role = 'payer'
		    UNION ALL
		    SELECT 'known', txn_count, volume_usdc FROM entity_day_v1 WHERE role = 'payer' AND facilitator_known
		) t GROUP BY lens`)
	if err != nil {
		return nil, fmt.Errorf("meta totals: %w", err)
	}
	defer rows.Close()
	out := map[string]LensTotals{"known": {VolumeUSDC: "0"}, "all": {VolumeUSDC: "0"}}
	for rows.Next() {
		var lens string
		var t LensTotals
		if err := rows.Scan(&lens, &t.TxnCount, &t.VolumeUSDC); err != nil {
			return nil, fmt.Errorf("scan meta totals: %w", err)
		}
		out[lens] = t
	}
	return out, rows.Err()
}
