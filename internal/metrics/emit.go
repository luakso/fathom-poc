package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukostrobl/fathom/web"
)

// artifact is the envelope every emitted file shares: data plus provenance
// stamps for citability and staleness display.
type artifact struct {
	MethodologyVersion int    `json:"methodology_version"`
	GeneratedAt        string `json:"generated_at"`
	DataThroughDay     string `json:"data_through_day"`
	Data               any    `json:"data"`
}

// Emit builds every page and writes it as <page>.json under outDir. claims is
// the curated, already-validated ledger (LoadClaims); it is joined to measured
// numbers here so re-emitting after a ledger edit costs no table scan.
//
// All reads run inside one REPEATABLE READ transaction so the stamps and every
// page come from the same cube snapshot — a rollup committing mid-emit cannot
// produce artifacts that disagree with their own data_through_day.
//
// Windows are anchored to the cube's data_through_day, not the wall clock: the
// dataset is static between deliberate backfills, so "7d" means the last 7 days
// of data, and re-emitting later never flatlines the windows. An empty cube is
// an error (run `publisher rollup` first), never an all-zero artifact.
func Emit(ctx context.Context, pool *pgxpool.Pool, outDir string, claims []Claim) error {
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
	if econ.Claims, err = ResolveClaims(econ, claims); err != nil {
		return fmt.Errorf("resolve claims: %w", err)
	}
	if econ.Concentration, err = BuildConcentration(ctx, tx); err != nil {
		return fmt.Errorf("build concentration: %w", err)
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
	payees, err := BuildEntities(ctx, tx, "payee")
	if err != nil {
		return fmt.Errorf("build payees: %w", err)
	}
	if err := writeArtifact(outDir, "payees.json", version, through, payees); err != nil {
		return err
	}
	payers, err := BuildEntities(ctx, tx, "payer")
	if err != nil {
		return fmt.Errorf("build payers: %w", err)
	}
	if err := writeArtifact(outDir, "payers.json", version, through, payers); err != nil {
		return err
	}
	reliability, err := BuildReliability(ctx, tx)
	if err != nil {
		return fmt.Errorf("build reliability: %w", err)
	}
	if err := writeArtifact(outDir, "reliability.json", version, through, reliability); err != nil {
		return err
	}
	mechanics, err := BuildMechanics(ctx, tx)
	if err != nil {
		return fmt.Errorf("build mechanics: %w", err)
	}
	if err := writeArtifact(outDir, "mechanics.json", version, through, mechanics); err != nil {
		return err
	}
	if err := writeSite(outDir); err != nil {
		return fmt.Errorf("write site: %w", err)
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
		SELECT
		    (SELECT max(day)::text FROM metrics_daily_v2),
		    count(DISTINCT methodology_version),
		    min(methodology_version)
		FROM (
		    SELECT methodology_version FROM metrics_daily_v2
		    UNION ALL SELECT methodology_version FROM metrics_window_stats_v2
		    UNION ALL SELECT methodology_version FROM metrics_price_points_v2
		    UNION ALL SELECT methodology_version FROM metrics_gas_daily_v2
		    UNION ALL SELECT methodology_version FROM metrics_velocity_daily_v2
		    UNION ALL SELECT methodology_version FROM entity_rank_v1
		    UNION ALL SELECT methodology_version FROM entity_buckets_v1
		    UNION ALL SELECT methodology_version FROM entity_concentration_v1
		    UNION ALL SELECT methodology_version FROM metrics_reliability_window_v2
		    UNION ALL SELECT methodology_version FROM metrics_reliability_daily_v2
		    UNION ALL SELECT methodology_version FROM metrics_mechanics_window_v2
		    UNION ALL SELECT methodology_version FROM metrics_mechanics_batch_v2
		    UNION ALL SELECT methodology_version FROM metrics_mechanics_block_v2
		    UNION ALL SELECT methodology_version FROM metrics_mechanics_selector_v2
		) versions`).Scan(&day, &versions, &minVersion); err != nil {
		return "", 0, fmt.Errorf("cube stamp: %w", err)
	}
	if day == nil {
		return "", 0, errors.New("metrics_daily_v2 is empty — run `publisher rollup` before emit; refusing to overwrite artifacts with zeros")
	}
	if versions != 1 {
		return "", 0, fmt.Errorf("cube stamp: expected one methodology_version across metrics tables, found %d — rebuild the cube", versions)
	}
	return *day, int(*minVersion), nil
}

// writeArtifact writes a stamped JSON document via writeFileAtomic.
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
	return writeFileAtomic(outDir, name, b)
}

// writeFileAtomic writes via temp file + rename so a reader of the live,
// statically-served directory never sees a truncated file. name may contain
// subdirectories (created as needed).
func writeFileAtomic(outDir, name string, b []byte) error {
	dst := filepath.Join(outDir, name)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { //nolint:gosec // G301: public static site
		return fmt.Errorf("mkdir for %s: %w", name, err)
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil { //nolint:gosec // G306: public static site
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s into place: %w", name, err)
	}
	return nil
}

// writeSite copies the embedded dashboard into outDir. It runs AFTER the JSON
// artifacts so a mid-failure never leaves a page pointing at missing data.
// Files under assets/ that the embed no longer ships are pruned, so renames
// and deletes converge instead of accumulating stale modules in the served
// directory. Only assets/ is pruned: the outDir root also holds the JSON
// artifacts, which are not the site's to manage.
func writeSite(outDir string) error {
	shipped := map[string]bool{}
	err := fs.WalkDir(web.Site, "site", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk embedded site: %w", err)
		}
		if d.IsDir() {
			return nil
		}
		b, err := web.Site.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		rel := strings.TrimPrefix(path, "site/")
		shipped[rel] = true
		return writeFileAtomic(outDir, rel, b)
	})
	if err != nil {
		return err
	}
	return pruneStale(outDir, "assets", shipped)
}

// pruneStale removes files under outDir/sub that are not in shipped (keyed by
// slash-separated path relative to outDir).
func pruneStale(outDir, sub string, shipped map[string]bool) error {
	return filepath.WalkDir(filepath.Join(outDir, sub), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // first emit: nothing to prune
			}
			return fmt.Errorf("prune %s: %w", sub, err)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(outDir, path)
		if err != nil {
			return fmt.Errorf("prune %s: %w", sub, err)
		}
		if !shipped[filepath.ToSlash(rel)] {
			//nolint:gosec // G122: pruning the publisher's own outDir/assets tree (paths walked from it), not attacker-controlled input — symlink TOCTOU is not a threat here.
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove stale %s: %w", rel, err)
			}
		}
		return nil
	})
}
