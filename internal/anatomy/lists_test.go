//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestCounterparties_SortAndPagination(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	require.NoError(t, anatomy.Rollup(ctx, pool, nil))
	pg := anatomy.NewPgEntity(pool)

	page, err := pg.Counterparties(ctx, "base", "0xp1", anatomy.CounterpartyQuery{
		Role: "payer", Lens: "known", Sort: "volume", Limit: 1, Offset: 0,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), page.Total)
	require.Len(t, page.Rows, 1)
	require.Equal(t, "0xe1", page.Rows[0].Address)

	page, err = pg.Counterparties(ctx, "base", "0xp1", anatomy.CounterpartyQuery{
		Role: "payer", Lens: "known", Sort: "volume", Limit: 1, Offset: 1,
	})
	require.NoError(t, err)
	require.Equal(t, "0xe2", page.Rows[0].Address)

	// facilitator subject lists merged counterparties.
	page, err = pg.Counterparties(ctx, "base", "0xkfac", anatomy.CounterpartyQuery{
		Role: "facilitator", Lens: "known", Sort: "txns", Limit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(3), page.Total) // p1, e1, e2
}

func TestPayments_KeysetPagination(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db) // block_number 100 for all; keyset falls back to tx_hash ordering
	require.NoError(t, anatomy.Rollup(ctx, pool, nil))
	pg := anatomy.NewPgEntity(pool)

	p1, err := pg.Payments(ctx, "base", "0xp1", anatomy.PaymentQuery{Role: "payer", Lens: "known", Limit: 2})
	require.NoError(t, err)
	require.Len(t, p1.Rows, 2)
	require.NotEmpty(t, p1.Next)

	p2, err := pg.Payments(ctx, "base", "0xp1", anatomy.PaymentQuery{Role: "payer", Lens: "known", Limit: 2, Before: p1.Next})
	require.NoError(t, err)
	require.Len(t, p2.Rows, 1)
	require.Empty(t, p2.Next)
	// No overlap between pages.
	seen := map[string]bool{}
	for _, r := range append(p1.Rows, p2.Rows...) {
		key := r.TxHash
		require.False(t, seen[key], "duplicate row across pages: %s", key)
		seen[key] = true
	}

	// lens=all for p2 includes unknown-facilitator payments.
	pAll, err := pg.Payments(ctx, "base", "0xp2", anatomy.PaymentQuery{Role: "payer", Lens: "all", Limit: 10})
	require.NoError(t, err)
	require.Len(t, pAll.Rows, 2)
	pKnown, err := pg.Payments(ctx, "base", "0xp2", anatomy.PaymentQuery{Role: "payer", Lens: "known", Limit: 10})
	require.NoError(t, err)
	require.Len(t, pKnown.Rows, 0)
}
