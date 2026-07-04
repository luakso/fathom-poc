//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestMeta_TotalsAndStamp(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	require.NoError(t, anatomy.Rollup(ctx, pool, nil))

	m, err := anatomy.NewPgMeta(pool).Meta(ctx)
	require.NoError(t, err)
	require.Equal(t, "2026-06-03", m.DataMaxDay)
	require.Equal(t, 1, m.MethodologyVersion)
	require.Equal(t, int64(3), m.Totals["known"].TxnCount)
	require.Equal(t, "10.000000", m.Totals["known"].VolumeUSDC)
	require.Equal(t, int64(5), m.Totals["all"].TxnCount)
	require.Equal(t, "28.000000", m.Totals["all"].VolumeUSDC)

	// Cached: a second call returns the same without error.
	m2, err := anatomy.NewPgMeta(pool).Meta(ctx)
	require.NoError(t, err)
	require.Equal(t, m, m2)
}

func TestMeta_EmptyDB(t *testing.T) {
	ctx, _, pool := setupAnatomy(t)
	_, err := anatomy.NewPgMeta(pool).Meta(ctx)
	require.ErrorIs(t, err, anatomy.ErrNotFound) // no rollup yet -> 404 semantics
}
