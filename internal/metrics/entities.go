package metrics

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// entityRoles maps a role to its (entity column, counterparty column) in
// payment_x402_v1. role is a fixed internal constant, never user input, so it is
// safe to interpolate into SQL.
var entityRoles = []struct{ role, entityCol, counterpartyCol string }{
	{"payee", "payee", "payer"},
	{"payer", "payer", "payee"},
}

// RebuildEntities recomputes the three entity tables for every (window, role)
// from payment_x402_v1. Called by Rebuild inside its REPEATABLE READ transaction
// AFTER the cube statements, so all artifacts share one snapshot. Per role it
// materializes a per-(window,entity) aggregate into a temp table (one scan), then
// derives the leaderboard union, the bucket histogram, and the concentration
// summary from it. Exact distinct counts (no HLL). The tx's temp_file_limit caps
// the count(DISTINCT) and window-sort spill (the dominant cost); entity_agg itself
// is small (~437k entities x <=3 windows).
func RebuildEntities(ctx context.Context, tx pgx.Tx) error {
	for _, t := range []string{"entity_rank_v1", "entity_buckets_v1", "entity_concentration_v1"} {
		if _, err := tx.Exec(ctx, "TRUNCATE "+t); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	for _, r := range entityRoles {
		if err := rebuildEntityRole(ctx, tx, r.role, r.entityCol, r.counterpartyCol); err != nil {
			return fmt.Errorf("rebuild entities %s: %w", r.role, err)
		}
	}
	return nil
}

// rebuildEntityRole builds a fresh entity_agg temp table for one role and inserts
// the three derived projections. entity_agg is dropped at the end so the next role
// recreates it; the surrounding tx already sets temp_file_limit.
func rebuildEntityRole(ctx context.Context, tx pgx.Tx, role, entityCol, counterpartyCol string) error {
	if _, err := tx.Exec(ctx, "DROP TABLE IF EXISTS entity_agg"); err != nil {
		return fmt.Errorf("drop stale entity_agg: %w", err)
	}

	// NB first_seen/last_seen are within-window (only 'all' is lifetime).
	aggSQL := fmt.Sprintf(`
CREATE TEMP TABLE entity_agg AS
SELECT
    w.window_name,
    p.%[1]s                                   AS address,
    sum(p.amount_usdc)                        AS volume_usdc,
    count(*)                                  AS txn_count,
    count(DISTINCT p.%[2]s)                   AS distinct_counterparties,
    count(DISTINCT p.amount_usdc)             AS distinct_amounts,
    COALESCE(sum(p.amount_usdc) FILTER (WHERE p.facilitator_known), 0) AS known_volume_usdc,
    min(p.block_timestamp)                    AS first_seen,
    max(p.block_timestamp)                    AS last_seen,
    min(p.methodology_version)                AS methodology_version
FROM payment_x402_v1 p
CROSS JOIN %[3]s
CROSS JOIN (SELECT max((block_timestamp AT TIME ZONE 'UTC')::date) AS d FROM payment_x402_v1) anchor
WHERE w.days = 0
   OR (p.block_timestamp AT TIME ZONE 'UTC')::date >= anchor.d - (w.days - 1)
GROUP BY w.window_name, p.%[1]s`, entityCol, counterpartyCol, windowsValues)

	if _, err := tx.Exec(ctx, aggSQL); err != nil {
		return fmt.Errorf("build entity_agg: %w", err)
	}

	// Leaderboard: union of top-150-by-volume and top-150-by-txns per window.
	rankSQL := fmt.Sprintf(`
INSERT INTO entity_rank_v1
    (window_name, role, address, volume_usdc, txn_count, distinct_counterparties,
     distinct_amounts, known_volume_usdc, first_seen, last_seen, methodology_version)
SELECT window_name, '%[1]s', address, volume_usdc, txn_count, distinct_counterparties,
       distinct_amounts, known_volume_usdc, first_seen, last_seen, methodology_version
FROM (
    SELECT *,
        row_number() OVER (PARTITION BY window_name ORDER BY volume_usdc DESC, address) AS rv,
        row_number() OVER (PARTITION BY window_name ORDER BY txn_count  DESC, address) AS rt
    FROM entity_agg
) ranked
WHERE rv <= 150 OR rt <= 150`, role)
	if _, err := tx.Exec(ctx, rankSQL); err != nil {
		return fmt.Errorf("insert entity_rank: %w", err)
	}

	bucketSQL := fmt.Sprintf(`
INSERT INTO entity_buckets_v1
    (window_name, role, bucket, entity_count, txn_sum, volume_sum, methodology_version)
SELECT window_name, '%[1]s', entity_txn_bucket(txn_count),
       count(*), sum(txn_count), sum(volume_usdc), min(methodology_version)
FROM entity_agg
GROUP BY window_name, entity_txn_bucket(txn_count)`, role)
	if _, err := tx.Exec(ctx, bucketSQL); err != nil {
		return fmt.Errorf("insert entity_buckets: %w", err)
	}

	concSQL := fmt.Sprintf(`
INSERT INTO entity_concentration_v1
    (window_name, role, total_entities, total_volume, total_txns,
     top10_volume, top10_txns, top100_volume, methodology_version)
SELECT window_name, '%[1]s',
       count(*),
       sum(volume_usdc),
       sum(txn_count),
       COALESCE(sum(volume_usdc) FILTER (WHERE rv <= 10), 0),
       COALESCE(sum(txn_count)   FILTER (WHERE rt <= 10), 0),
       COALESCE(sum(volume_usdc) FILTER (WHERE rv <= 100), 0),
       min(methodology_version)
FROM (
    SELECT window_name, volume_usdc, txn_count, methodology_version,
        row_number() OVER (PARTITION BY window_name ORDER BY volume_usdc DESC, address) AS rv,
        row_number() OVER (PARTITION BY window_name ORDER BY txn_count  DESC, address) AS rt
    FROM entity_agg
) ranked
GROUP BY window_name`, role)
	if _, err := tx.Exec(ctx, concSQL); err != nil {
		return fmt.Errorf("insert entity_concentration: %w", err)
	}

	if _, err := tx.Exec(ctx, "DROP TABLE entity_agg"); err != nil {
		return fmt.Errorf("drop entity_agg: %w", err)
	}
	return nil
}

// EntityRow is one ranked entity (the leaderboard union row). Decimals are
// strings to preserve exactness through JSON.
type EntityRow struct {
	Address                string `json:"address"`
	VolumeUSDC             string `json:"volume_usdc"`
	TxnCount               int64  `json:"txn_count"`
	DistinctCounterparties int64  `json:"distinct_counterparties"`
	DistinctAmounts        int64  `json:"distinct_amounts"`
	KnownVolumeUSDC        string `json:"known_volume_usdc"`
	FirstSeen              string `json:"first_seen"`
	LastSeen               string `json:"last_seen"`
}

// EntityBucket is one Y2 activity-histogram bar.
type EntityBucket struct {
	Bucket      string `json:"bucket"`
	EntityCount int64  `json:"entity_count"`
	TxnSum      int64  `json:"txn_sum"`
	VolumeSum   string `json:"volume_sum"`
}

// EntityConcentration is the P11/E9 summary for one (window, role).
type EntityConcentration struct {
	TotalEntities int64  `json:"total_entities"`
	TotalVolume   string `json:"total_volume"`
	TotalTxns     int64  `json:"total_txns"`
	Top10Volume   string `json:"top10_volume"`
	Top10Txns     int64  `json:"top10_txns"`
	Top100Volume  string `json:"top100_volume"`
}

// EntityWindow is one window's slice of an entity page.
type EntityWindow struct {
	Leaderboard   []EntityRow         `json:"leaderboard"`
	Buckets       []EntityBucket      `json:"buckets"`
	Concentration EntityConcentration `json:"concentration"`
}

// EntityPage is the payees.json / payers.json payload.
type EntityPage struct {
	Role    string                  `json:"role"`
	Windows map[string]EntityWindow `json:"windows"`
}

// ConcentrationSection is the E9 add-on for economy.json: window -> role -> conc.
type ConcentrationSection struct {
	Windows map[string]map[string]EntityConcentration `json:"windows"`
}

// BuildEntities assembles one role's entity page from the three entity tables.
// role is 'payee' or 'payer'. Windows absent from the tables come back empty.
func BuildEntities(ctx context.Context, q Querier, role string) (EntityPage, error) {
	page := EntityPage{Role: role, Windows: map[string]EntityWindow{}}
	zeroConc := EntityConcentration{TotalVolume: "0", Top10Volume: "0", Top100Volume: "0"}
	for w := range windowDays {
		page.Windows[w] = EntityWindow{
			Leaderboard:   []EntityRow{},
			Buckets:       []EntityBucket{},
			Concentration: zeroConc,
		}
	}

	rrows, err := q.Query(ctx, `
		SELECT window_name, address, volume_usdc::text, txn_count, distinct_counterparties,
		       distinct_amounts, known_volume_usdc::text, first_seen::text, last_seen::text
		FROM entity_rank_v1 WHERE role = $1
		ORDER BY window_name, entity_rank_v1.volume_usdc DESC, address`, role)
	if err != nil {
		return EntityPage{}, fmt.Errorf("entity rank read: %w", err)
	}
	defer rrows.Close()
	for rrows.Next() {
		var w string
		var r EntityRow
		if err := rrows.Scan(&w, &r.Address, &r.VolumeUSDC, &r.TxnCount, &r.DistinctCounterparties,
			&r.DistinctAmounts, &r.KnownVolumeUSDC, &r.FirstSeen, &r.LastSeen); err != nil {
			return EntityPage{}, fmt.Errorf("scan entity rank: %w", err)
		}
		ew, ok := page.Windows[w]
		if !ok {
			return EntityPage{}, fmt.Errorf("entity rank: unknown window %q", w)
		}
		ew.Leaderboard = append(ew.Leaderboard, r)
		page.Windows[w] = ew
	}
	if err := rrows.Err(); err != nil {
		return EntityPage{}, fmt.Errorf("entity rank read: %w", err)
	}

	brows, err := q.Query(ctx, `
		SELECT window_name, bucket, entity_count, txn_sum, volume_sum::text
		FROM entity_buckets_v1 WHERE role = $1
		ORDER BY window_name, bucket`, role)
	if err != nil {
		return EntityPage{}, fmt.Errorf("entity buckets read: %w", err)
	}
	defer brows.Close()
	for brows.Next() {
		var w string
		var b EntityBucket
		if err := brows.Scan(&w, &b.Bucket, &b.EntityCount, &b.TxnSum, &b.VolumeSum); err != nil {
			return EntityPage{}, fmt.Errorf("scan entity bucket: %w", err)
		}
		ew, ok := page.Windows[w]
		if !ok {
			return EntityPage{}, fmt.Errorf("entity buckets: unknown window %q", w)
		}
		ew.Buckets = append(ew.Buckets, b)
		page.Windows[w] = ew
	}
	if err := brows.Err(); err != nil {
		return EntityPage{}, fmt.Errorf("entity buckets read: %w", err)
	}

	conc, err := readConcentration(ctx, q, role)
	if err != nil {
		return EntityPage{}, err
	}
	for w, c := range conc {
		ew, ok := page.Windows[w]
		if !ok {
			return EntityPage{}, fmt.Errorf("entity concentration: unknown window %q", w)
		}
		ew.Concentration = c
		page.Windows[w] = ew
	}
	return page, nil
}

// readConcentration returns window -> EntityConcentration for one role.
func readConcentration(ctx context.Context, q Querier, role string) (map[string]EntityConcentration, error) {
	rows, err := q.Query(ctx, `
		SELECT window_name, total_entities, total_volume::text, total_txns,
		       top10_volume::text, top10_txns, top100_volume::text
		FROM entity_concentration_v1 WHERE role = $1`, role)
	if err != nil {
		return nil, fmt.Errorf("concentration read: %w", err)
	}
	defer rows.Close()
	out := map[string]EntityConcentration{}
	for rows.Next() {
		var w string
		var c EntityConcentration
		if err := rows.Scan(&w, &c.TotalEntities, &c.TotalVolume, &c.TotalTxns,
			&c.Top10Volume, &c.Top10Txns, &c.Top100Volume); err != nil {
			return nil, fmt.Errorf("scan concentration: %w", err)
		}
		out[w] = c
	}
	return out, rows.Err()
}

// BuildConcentration assembles the economy.json E9 section (both roles).
func BuildConcentration(ctx context.Context, q Querier) (ConcentrationSection, error) {
	sec := ConcentrationSection{Windows: map[string]map[string]EntityConcentration{}}
	for _, role := range []string{"payee", "payer"} {
		conc, err := readConcentration(ctx, q, role)
		if err != nil {
			return ConcentrationSection{}, err
		}
		for w, c := range conc {
			if sec.Windows[w] == nil {
				sec.Windows[w] = map[string]EntityConcentration{}
			}
			sec.Windows[w][role] = c
		}
	}
	return sec, nil
}
