//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestEntity_HeaderBothLenses(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	labels := []anatomy.ManualLabel{{Chain: "base", Address: "0xe1", Label: "api.example.com", URL: "https://example.com"}}
	require.NoError(t, anatomy.Rollup(ctx, pool, labels))

	// p1: pure payer, 3 known payments over 2 days, 2 counterparties.
	e, err := anatomy.NewPgEntity(pool).Entity(ctx, "base", "0xp1")
	require.NoError(t, err)
	require.Equal(t, []string{"payer"}, e.Roles)
	known := e.Summaries["payer"]["known"]
	require.Equal(t, int64(3), known.TxnCount)
	require.Equal(t, "10.000000", known.VolumeUSDC)
	require.Equal(t, int64(2), known.ActiveDays)
	require.Equal(t, "2026-06-01", known.FirstDay)
	require.Equal(t, "2026-06-02", known.LastDay)
	require.Equal(t, int64(2), known.DistinctCounterparties)
	all := e.Summaries["payer"]["all"]
	require.Equal(t, int64(3), all.TxnCount) // p1 has no unknown-lens payments

	// e1: payee with mixed lenses ($2+$5 known, $7 unknown) and a manual label.
	e, err = anatomy.NewPgEntity(pool).Entity(ctx, "base", "0xe1")
	require.NoError(t, err)
	require.Equal(t, []string{"payee"}, e.Roles)
	require.Equal(t, "api.example.com", e.Label)
	require.Equal(t, "manual", e.LabelSource)
	require.Equal(t, "7.000000", e.Summaries["payee"]["known"].VolumeUSDC)
	require.Equal(t, "14.000000", e.Summaries["payee"]["all"].VolumeUSDC)
	require.Equal(t, int64(1), e.Summaries["payee"]["known"].DistinctCounterparties) // p1 only
	require.Equal(t, int64(2), e.Summaries["payee"]["all"].DistinctCounterparties)   // p1, p2

	// 0xkfac: facilitator role present with correct totals.
	e, err = anatomy.NewPgEntity(pool).Entity(ctx, "base", "0xkfac")
	require.NoError(t, err)
	require.Contains(t, e.Roles, "facilitator")
	require.Equal(t, int64(3), e.Summaries["facilitator"]["known"].TxnCount)

	// Unknown address -> ErrNotFound.
	_, err = anatomy.NewPgEntity(pool).Entity(ctx, "base", "0x00000000000000000000000000000000000000aa")
	require.ErrorIs(t, err, anatomy.ErrNotFound)
}
