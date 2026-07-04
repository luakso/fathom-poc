//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestLeaderboard_SortLensWindow(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	labels := []anatomy.ManualLabel{{Chain: "base", Address: "0xe2", Label: "labeled.example"}}
	require.NoError(t, anatomy.Rollup(ctx, pool, labels))
	pg := anatomy.NewPgLeaderboard(pool)

	// payee / all window / all lens by volume: e2 ($14) then e1 ($14)? -> check:
	// e1 = t1 $2 + t3 $5 + t4 $7 = $14; e2 = t2 $3 + t5 $11 = $14. Tie on volume,
	// address ascending breaks it: 0xe1 before 0xe2.
	lb, err := pg.Leaderboard(ctx, "base", "payee", "all", "all", "volume")
	require.NoError(t, err)
	require.Len(t, lb.Rows, 2)
	require.Equal(t, "0xe1", lb.Rows[0].Address)
	require.Equal(t, 1, lb.Rows[0].Rank)
	require.Equal(t, "14.000000", lb.Rows[0].VolumeUSDC)
	require.Equal(t, "labeled.example", lb.Rows[1].Label)

	// known lens: only known-fac payments count; e1 $7 > e2 $3.
	lb, err = pg.Leaderboard(ctx, "base", "payee", "all", "known", "volume")
	require.NoError(t, err)
	require.Equal(t, "0xe1", lb.Rows[0].Address)
	require.Equal(t, "7.000000", lb.Rows[0].VolumeUSDC)

	// sort=txns puts the higher-count payee first.
	lb, err = pg.Leaderboard(ctx, "base", "payee", "all", "all", "txns")
	require.NoError(t, err)
	require.Equal(t, "0xe1", lb.Rows[0].Address) // 3 txns vs 2
	require.Equal(t, int64(3), lb.Rows[0].TxnCount)
}
