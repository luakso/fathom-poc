package base

import (
	"context"
	"fmt"
	"io"
)

// StatusReport is the operator-facing snapshot. Aimed at the spec's 30-min/week
// budget — most of that budget is glancing at this output and comparing
// against x402scan/Dune.
type StatusReport struct {
	Cursor               uint64
	RowsLast24h          int64
	RowsLast7d           int64
	DistinctFacilitators int64
}

// RunStatus reads the cursor and aggregates the payments table. No RPC calls
// — chain-tip comparison is intentionally omitted to keep status cheap and
// runnable without RPC credentials.
func RunStatus(ctx context.Context, store *Store) (StatusReport, error) {
	cur, err := store.GetCursor(ctx)
	if err != nil {
		return StatusReport{}, fmt.Errorf("read cursor: %w", err)
	}
	report := StatusReport{Cursor: cur}

	pool := store.Pool()

	if err := pool.QueryRow(
		ctx,
		`SELECT count(*) FROM payments WHERE block_timestamp > now() - interval '24 hours'`,
	).Scan(&report.RowsLast24h); err != nil {
		return StatusReport{}, fmt.Errorf("count 24h: %w", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT count(*) FROM payments WHERE block_timestamp > now() - interval '7 days'`,
	).Scan(&report.RowsLast7d); err != nil {
		return StatusReport{}, fmt.Errorf("count 7d: %w", err)
	}
	if err := pool.QueryRow(
		ctx,
		`SELECT count(DISTINCT facilitator) FROM payments`,
	).Scan(&report.DistinctFacilitators); err != nil {
		return StatusReport{}, fmt.Errorf("count facilitators: %w", err)
	}
	return report, nil
}

// Print writes the report in a human-readable form.
func (r StatusReport) Print(w io.Writer) {
	_, _ = fmt.Fprintf(w, "base-collector status\n")
	_, _ = fmt.Fprintf(w, "  cursor (last_block):       %d\n", r.Cursor)
	_, _ = fmt.Fprintf(w, "  rows in last 24h:          %d\n", r.RowsLast24h)
	_, _ = fmt.Fprintf(w, "  rows in last 7d:           %d\n", r.RowsLast7d)
	_, _ = fmt.Fprintf(w, "  distinct facilitators:     %d\n", r.DistinctFacilitators)
}
