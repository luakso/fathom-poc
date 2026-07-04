//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestNeighbors_DirectionsAndShares(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	labels := []anatomy.ManualLabel{{Chain: "base", Address: "0xe1", Label: "api.example.com"}}
	require.NoError(t, anatomy.Rollup(ctx, pool, labels))
	pg := anatomy.NewPgEntity(pool)

	// p1 under known lens: pays e1 ($7) and e2 ($3); facilitator 0xkfac.
	n, err := pg.Neighbors(ctx, "base", "0xp1", "known", 8)
	require.NoError(t, err)
	require.Equal(t, "known", n.Lens)
	require.NotNil(t, n.Payees)
	require.Equal(t, int64(2), n.Payees.Total)
	require.Equal(t, "0xe1", n.Payees.Rows[0].Address) // top by volume
	require.Equal(t, "api.example.com", n.Payees.Rows[0].Label)
	require.Equal(t, "7.000000", n.Payees.Rows[0].VolumeUSDC)
	require.Equal(t, "0.700000", n.Payees.Rows[0].Share)
	require.NotNil(t, n.Facilitators)
	require.Equal(t, "0xkfac", n.Facilitators.Rows[0].Address)
	require.Nil(t, n.Payers)        // p1 receives nothing
	require.Nil(t, n.SettledPayers) // p1 facilitates nothing

	// e1 under all lens: paid by p1 ($7) and p2 ($7).
	n, err = pg.Neighbors(ctx, "base", "0xe1", "all", 8)
	require.NoError(t, err)
	require.Equal(t, int64(2), n.Payers.Total)
	require.Len(t, n.Payers.Rows, 2)
	require.Equal(t, "0.500000", n.Payers.Rows[0].Share)

	// limit=1 truncates rows but Total stays honest.
	n, err = pg.Neighbors(ctx, "base", "0xe1", "all", 1)
	require.NoError(t, err)
	require.Len(t, n.Payers.Rows, 1)
	require.Equal(t, int64(2), n.Payers.Total)

	// Facilitator subject: settled payers/payees populated.
	n, err = pg.Neighbors(ctx, "base", "0xkfac", "known", 8)
	require.NoError(t, err)
	require.Equal(t, int64(1), n.SettledPayers.Total) // p1
	require.Equal(t, int64(2), n.SettledPayees.Total) // e1, e2

	// Unknown address -> ErrNotFound.
	_, err = pg.Neighbors(ctx, "base", "0x00000000000000000000000000000000000000aa", "known", 8)
	require.ErrorIs(t, err, anatomy.ErrNotFound)
}
