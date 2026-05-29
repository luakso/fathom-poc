package base

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// These cover the pure confirmation-depth + batch-clamp math without a live
// RPC or database, so the trickiest boundaries run under plain `go test`.
func TestNextWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                              string
		cursor, tip, batchSize, confDepth uint64
		wantFrom, wantTo                  uint64
		wantWork                          bool
	}{
		{
			name:   "chain younger than confirmation depth",
			cursor: 0, tip: 3, batchSize: 100, confDepth: 6,
			wantWork: false,
		},
		{
			name:   "tip exactly equals confirmation depth (safeTip 0, cursor 0)",
			cursor: 0, tip: 6, batchSize: 100, confDepth: 6,
			wantWork: false, // safeTip 0, cursor 0 >= 0 -> caught up
		},
		{
			name:   "caught up: cursor equals safeTip",
			cursor: 294, tip: 300, batchSize: 50, confDepth: 6,
			wantWork: false,
		},
		{
			name:   "one block of work: cursor is safeTip-1",
			cursor: 293, tip: 300, batchSize: 50, confDepth: 6,
			wantFrom: 294, wantTo: 294, wantWork: true,
		},
		{
			name:   "full batch from genesis",
			cursor: 0, tip: 300, batchSize: 50, confDepth: 6,
			wantFrom: 1, wantTo: 50, wantWork: true,
		},
		{
			name:   "batch clamped to safeTip near the head",
			cursor: 250, tip: 300, batchSize: 50, confDepth: 6,
			wantFrom: 251, wantTo: 294, wantWork: true, // 251+50-1=300 clamped to 294
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			from, to, work := nextWindow(tc.cursor, tc.tip, tc.batchSize, tc.confDepth)
			require.Equal(t, tc.wantWork, work)
			if tc.wantWork {
				require.Equal(t, tc.wantFrom, from, "from")
				require.Equal(t, tc.wantTo, to, "to")
				require.LessOrEqual(t, to, tc.tip-tc.confDepth, "to must never exceed safeTip")
				require.Equal(t, tc.cursor+1, from, "from must be exactly one past the cursor")
			}
		})
	}
}
