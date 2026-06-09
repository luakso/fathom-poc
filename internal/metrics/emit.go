package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// methodologyVersion stamps every artifact. Phase 1a reads payment_classified_v1
// (methodology v1). Bump alongside the view version.
const methodologyVersion = 1

// artifact is the envelope every emitted file shares: data plus provenance
// stamps for citability and staleness display.
type artifact struct {
	MethodologyVersion int    `json:"methodology_version"`
	GeneratedAt        string `json:"generated_at"`
	DataThroughDay     string `json:"data_through_day"`
	Data               any    `json:"data"`
}

// Emit builds every page and writes it as <page>.json under outDir. asOf pins
// the window math (pass time.Now().UTC() in production).
func Emit(ctx context.Context, pool *pgxpool.Pool, outDir string, asOf time.Time) error {
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	through, err := dataThroughDay(ctx, pool)
	if err != nil {
		return err
	}

	econ, err := BuildEconomy(ctx, pool, asOf)
	if err != nil {
		return fmt.Errorf("build economy: %w", err)
	}
	if err := writeArtifact(outDir, "economy.json", through, econ); err != nil {
		return err
	}

	fac, err := BuildFacilitators(ctx, pool, asOf)
	if err != nil {
		return fmt.Errorf("build facilitators: %w", err)
	}
	if err := writeArtifact(outDir, "facilitators.json", through, fac); err != nil {
		return err
	}
	return nil
}

// dataThroughDay reports the latest day present in the cube (empty if none).
func dataThroughDay(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var day *string
	if err := pool.QueryRow(ctx, `SELECT max(day)::text FROM metrics_daily_v1`).Scan(&day); err != nil {
		return "", fmt.Errorf("data_through_day: %w", err)
	}
	if day == nil {
		return "", nil
	}
	return *day, nil
}

func writeArtifact(outDir, name, through string, data any) error {
	doc := artifact{
		MethodologyVersion: methodologyVersion,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		DataThroughDay:     through,
		Data:               data,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	if err := os.WriteFile(filepath.Join(outDir, name), b, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}
