package anatomy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgLeaderboard reads the precomputed entity_leaderboard_v1 (top-500 union
// per window/role/lens, built by the rollup).
type PgLeaderboard struct{ pool *pgxpool.Pool }

// NewPgLeaderboard creates a new PgLeaderboard provider.
func NewPgLeaderboard(pool *pgxpool.Pool) *PgLeaderboard { return &PgLeaderboard{pool: pool} }

// lbSortCols whitelists sort expressions. Keys are the closed Sort set
// (validated by parseLeaderboardSort); values are never user input.
var lbSortCols = map[Sort]string{
	SortVolume:         "volume_usdc DESC",
	SortTxns:           "txn_count DESC",
	SortCounterparties: "distinct_counterparties DESC",
}

// Leaderboard implements LeaderboardProvider.
func (p *PgLeaderboard) Leaderboard(ctx context.Context, chain string, role Role, window Window, lens Lens, sortKey Sort) (Leaderboard, error) {
	order, ok := lbSortCols[sortKey]
	if !ok {
		return Leaderboard{}, fmt.Errorf("unknown sort %q", sortKey)
	}
	// entity_leaderboard_v1 and the entity_identity_v1 join are Base-only in v1
	// (the SQL hardcodes 'base'). Guard so adding a second chain trips loudly
	// here instead of silently returning Base rows under another chain's label.
	if chain != "base" {
		return Leaderboard{}, fmt.Errorf("leaderboard: unsupported chain %q", chain)
	}
	sql := fmt.Sprintf(`
		SELECT l.address, COALESCE(i.label, ''), l.txn_count, l.volume_usdc::text,
		       l.distinct_counterparties, l.first_seen::text, l.last_seen::text
		FROM entity_leaderboard_v1 l
		LEFT JOIN entity_identity_v1 i ON i.chain = 'base' AND i.address = l.address
		WHERE l.window_name = $1 AND l.role = $2 AND l.lens = $3
		ORDER BY l.%s, l.address
		LIMIT 500`, order)
	rows, err := p.pool.Query(ctx, sql, string(window), string(role), string(lens))
	if err != nil {
		return Leaderboard{}, fmt.Errorf("leaderboard: %w", err)
	}
	defer rows.Close()
	lb := Leaderboard{Role: role, Window: window, Lens: lens, Sort: sortKey}
	for rows.Next() {
		var r LeaderboardRow
		if err := rows.Scan(&r.Address, &r.Label, &r.TxnCount, &r.VolumeUSDC,
			&r.DistinctCounterparties, &r.FirstSeen, &r.LastSeen); err != nil {
			return Leaderboard{}, fmt.Errorf("scan leaderboard: %w", err)
		}
		r.Rank = len(lb.Rows) + 1
		lb.Rows = append(lb.Rows, r)
	}
	return lb, rows.Err()
}
