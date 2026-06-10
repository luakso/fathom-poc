package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// artifact is the envelope every emitted file shares: data plus provenance
// stamps for citability and staleness display.
type artifact struct {
	MethodologyVersion int    `json:"methodology_version"`
	GeneratedAt        string `json:"generated_at"`
	DataThroughDay     string `json:"data_through_day"`
	Data               any    `json:"data"`
}

// Emit builds every page and writes it as <page>.json under outDir.
//
// All reads run inside one REPEATABLE READ transaction so the stamps and every
// page come from the same cube snapshot — a rollup committing mid-emit cannot
// produce artifacts that disagree with their own data_through_day.
//
// Windows are anchored to the cube's data_through_day, not the wall clock: the
// dataset is static between deliberate backfills, so "7d" means the last 7 days
// of data, and re-emitting later never flatlines the windows. An empty cube is
// an error (run `publisher rollup` first), never an all-zero artifact.
func Emit(ctx context.Context, pool *pgxpool.Pool, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil { //nolint:gosec // G301: dashboard JSON is public, served as static files
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("begin emit snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	through, version, err := cubeStamp(ctx, tx)
	if err != nil {
		return err
	}
	asOf, err := time.Parse("2006-01-02", through)
	if err != nil {
		return fmt.Errorf("parse data_through_day %q: %w", through, err)
	}

	econ, err := BuildEconomy(ctx, tx, asOf)
	if err != nil {
		return fmt.Errorf("build economy: %w", err)
	}
	if err := writeArtifact(outDir, "economy.json", version, through, econ); err != nil {
		return err
	}

	fac, err := BuildFacilitators(ctx, tx)
	if err != nil {
		return fmt.Errorf("build facilitators: %w", err)
	}
	if err := writeArtifact(outDir, "facilitators.json", version, through, fac); err != nil {
		return err
	}
	return nil
}

// cubeStamp reads the provenance stamps off the cube itself: the latest day
// and the (asserted single) methodology version the rollup was built under.
func cubeStamp(ctx context.Context, q Querier) (through string, version int, err error) {
	var day *string
	var versions int64
	var minVersion *int16
	if err := q.QueryRow(ctx, `
		SELECT max(day)::text, count(DISTINCT methodology_version), min(methodology_version)
		FROM metrics_daily_v1`).Scan(&day, &versions, &minVersion); err != nil {
		return "", 0, fmt.Errorf("cube stamp: %w", err)
	}
	if day == nil {
		return "", 0, errors.New("metrics_daily_v1 is empty — run `publisher rollup` before emit; refusing to overwrite artifacts with zeros")
	}
	if versions != 1 {
		return "", 0, fmt.Errorf("cube stamp: expected one methodology_version in metrics_daily_v1, found %d — rebuild the cube", versions)
	}
	return *day, int(*minVersion), nil
}

// writeArtifact writes via temp file + rename so a reader of the live,
// statically-served directory never sees a truncated document.
func writeArtifact(outDir, name string, version int, through string, data any) error {
	doc := artifact{
		MethodologyVersion: version,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		DataThroughDay:     through,
		Data:               data,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	tmp := filepath.Join(outDir, name+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil { //nolint:gosec // G306: dashboard JSON is public, served as static files
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, filepath.Join(outDir, name)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s into place: %w", name, err)
	}
	return nil
}
