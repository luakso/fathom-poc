//go:build integration

package base_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/x402"
)

func TestRunStatus_ReportsCursorAndCounts(t *testing.T) {
	ctx, store := setupStore(t)

	// samplePayment hardcodes a 2023 BlockTimestamp; override to now so the
	// 24h/7d windows actually capture these rows.
	now := time.Now().UTC()
	p1 := samplePayment(1)
	p2 := samplePayment(2)
	p1.BlockTimestamp = now
	p2.BlockTimestamp = now

	require.NoError(t, store.InsertBatch(
		ctx,
		[]x402.Payment{p1, p2},
		nil,
		123,
	))

	r, err := base.RunStatus(ctx, store)
	require.NoError(t, err)
	require.Equal(t, uint64(123), r.Cursor)
	require.Equal(t, int64(2), r.RowsLast24h)
	require.GreaterOrEqual(t, r.DistinctFacilitators, int64(1))
}

func TestStatusReport_Print(t *testing.T) {
	t.Parallel()
	r := base.StatusReport{
		Cursor:               12345,
		RowsLast24h:          567,
		RowsLast7d:           4200,
		DistinctFacilitators: 22,
	}
	var buf bytes.Buffer
	r.Print(&buf)
	out := buf.String()
	require.Contains(t, out, "12345")
	require.Contains(t, out, "567")
	require.Contains(t, out, "4200")
	require.Contains(t, out, "22")
}
