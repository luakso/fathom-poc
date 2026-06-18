package anatomy

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStats aggregates per-address statistics from payment_x402_v1.
type PgStats struct{ pool *pgxpool.Pool }

var _ StatsProvider = (*PgStats)(nil)

// NewPgStats returns a PgStats reading from pool.
func NewPgStats(pool *pgxpool.Pool) *PgStats { return &PgStats{pool: pool} }

// Stats implements StatsProvider.
func (p *PgStats) Stats(ctx context.Context, chain, address string) (Stats, error) {
	addr := strings.ToLower(address)
	var (
		s                               Stats
		isPayer, isPayee, isFacilitator bool
	)
	err := p.pool.QueryRow(ctx, `
		WITH involved AS (
			SELECT * FROM payment_x402_v1
			WHERE chain = $1 AND (payer = $2 OR payee = $2 OR facilitator = $2)
		), counterparties AS (
			SELECT payee AS cp FROM involved WHERE payer = $2
			UNION
			SELECT payer AS cp FROM involved WHERE payee = $2
		)
		SELECT
			count(*),
			coalesce(sum(amount_usdc), 0)::text,
			coalesce(min(block_timestamp)::text, ''),
			coalesce(max(block_timestamp)::text, ''),
			coalesce(bool_or(facilitator_known AND facilitator = $2), false),
			coalesce(bool_or(payer = $2), false),
			coalesce(bool_or(payee = $2), false),
			coalesce(bool_or(facilitator = $2), false),
			(SELECT count(*) FROM counterparties)
		FROM involved`,
		chain, addr).Scan(
		&s.PaymentCount, &s.VolumeUSDC, &s.FirstSeen, &s.LastSeen,
		&s.FacilitatorKnown, &isPayer, &isPayee, &isFacilitator, &s.DistinctCounterparties,
	)
	if err != nil {
		return Stats{}, fmt.Errorf("query stats: %w", err)
	}
	if s.PaymentCount == 0 {
		return Stats{}, ErrNotFound
	}
	s.Address = addr
	s.Chain = chain
	if isPayer {
		s.Roles = append(s.Roles, RolePayer)
	}
	if isPayee {
		s.Roles = append(s.Roles, RolePayee)
	}
	if isFacilitator {
		s.Roles = append(s.Roles, RoleFacilitator)
	}
	return s, nil
}
