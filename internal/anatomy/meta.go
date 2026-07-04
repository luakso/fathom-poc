package anatomy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgMeta serves /api/meta from anatomy_meta plus lens totals aggregated from
// entity_day_v1 (payer role carries every payment exactly once). Totals are
// cached per built_at stamp: they only change when a rollup runs.
type PgMeta struct {
	pool *pgxpool.Pool

	mu     sync.Mutex
	cached Meta
	key    string // built_at of the cached totals
}

// NewPgMeta constructs a PgMeta backed by the given pool.
func NewPgMeta(pool *pgxpool.Pool) *PgMeta { return &PgMeta{pool: pool} }

// Meta returns the latest rollup metadata. Returns ErrNotFound when no rollup
// has been run yet (anatomy_meta has no rows).
// Concurrent /api/meta calls on a cache miss are serialized by the mutex;
// acceptable for this internal tool where rollup is rare and callers are few.
func (p *PgMeta) Meta(ctx context.Context) (Meta, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

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

	if p.key == builtAt {
		return p.cached, nil
	}
	totals, err := p.readTotals(ctx)
	if err != nil {
		return Meta{}, err
	}
	m.Totals = totals
	p.cached, p.key = m, builtAt
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
