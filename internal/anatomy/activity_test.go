//go:build integration

package anatomy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func TestTimeline_SparseSeriesPerRole(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	require.NoError(t, anatomy.Rollup(ctx, pool, nil))

	tl, err := anatomy.NewPgEntity(pool).Timeline(ctx, "base", "0xp1", "known")
	require.NoError(t, err)
	pts := tl.Roles["payer"]
	require.Len(t, pts, 2)
	require.Equal(t, "2026-06-01", pts[0].Day)
	require.Equal(t, int64(2), pts[0].TxnCount)
	require.Equal(t, "5.000000", pts[0].VolumeUSDC)
	require.Equal(t, "2026-06-02", pts[1].Day)

	// all lens merges facilitator_known slices per day.
	tl, err = anatomy.NewPgEntity(pool).Timeline(ctx, "base", "0xe1", "all")
	require.NoError(t, err)
	pts = tl.Roles["payee"]
	require.Len(t, pts, 2)
	require.Equal(t, "2.000000", pts[0].VolumeUSDC)  // 06-01
	require.Equal(t, "12.000000", pts[1].VolumeUSDC) // 06-02 known+unknown merged
	require.Equal(t, int64(2), pts[1].TxnCount)
}

func TestFingerprint_CadencePriceConcentration(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedRollupFixture(t, ctx, db)
	require.NoError(t, anatomy.Rollup(ctx, pool, nil))

	fp, err := anatomy.NewPgEntity(pool).Fingerprint(ctx, "base", "0xp1", "known")
	require.NoError(t, err)
	pr := fp.Roles["payer"]
	require.Equal(t, int64(2), pr.ActiveDays)
	require.Equal(t, int64(2), pr.SpanDays)
	require.Equal(t, int64(1), pr.MedianTxnsPerDay) // daily counts {2,1} -> lower median 1
	require.Equal(t, "0.666667", pr.TopDayShare)    // 2 of 3
	require.Len(t, pr.PricePoints, 3)               // $2, $3, $5 each once
	require.NotNil(t, pr.TotalDistinctAmounts)
	require.Equal(t, int64(3), *pr.TotalDistinctAmounts)
	require.Equal(t, "0.700000", pr.Top1Share) // e1 $7 of $10
	require.Equal(t, "1.000000", pr.Top3Share)

	// lens=all: TotalDistinctAmounts is null (cannot merge capped partitions).
	fp, err = anatomy.NewPgEntity(pool).Fingerprint(ctx, "base", "0xe1", "all")
	require.NoError(t, err)
	require.Nil(t, fp.Roles["payee"].TotalDistinctAmounts)
}
