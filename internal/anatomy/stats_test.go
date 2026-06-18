//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/lukostrobl/fathom/internal/anatomy"
	"github.com/stretchr/testify/require"
)

func TestStats_AggregatesAcrossRoles(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedPayment(t, ctx, db, "0xt1", 0, "0xhero", "0xfac", "0xshop", "2.00")
	seedPayment(t, ctx, db, "0xt2", 0, "0xhero", "0xfac", "0xcafe", "3.00")
	seedPayment(t, ctx, db, "0xt3", 0, "0xbob", "0xfac", "0xhero", "1.00") // hero as payee

	s, err := anatomy.NewPgStats(pool).Stats(ctx, "base", "0xhero")
	require.NoError(t, err)
	require.Equal(t, int64(3), s.PaymentCount)
	require.Equal(t, "6.000000", s.VolumeUSDC)
	require.Contains(t, s.Roles, anatomy.RolePayer)
	require.Contains(t, s.Roles, anatomy.RolePayee)
	// counterparties: 0xshop, 0xcafe (as payer) + 0xbob (as payee) = 3
	require.Equal(t, int64(3), s.DistinctCounterparties)
}

func TestStats_NotFound(t *testing.T) {
	ctx, _, pool := setupAnatomy(t)
	_, err := anatomy.NewPgStats(pool).Stats(ctx, "base", "0xnobody")
	require.ErrorIs(t, err, anatomy.ErrNotFound)
}
