package anatomy

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Timeline implements ActivityProvider: sparse per-role day series. Lens=all
// merges the two facilitator_known slices per (role, day); volume is summed
// in SQL to stay exact.
func (p *PgEntity) Timeline(ctx context.Context, chain, address string, lens Lens) (Timeline, error) {
	knownOnly := lens == LensKnown
	rows, err := p.pool.Query(ctx, `
		SELECT role, day::text, sum(txn_count), sum(volume_usdc)::text
		FROM entity_day_v1
		WHERE chain = $1 AND address = $2 AND ($3::boolean IS FALSE OR facilitator_known)
		GROUP BY role, day
		ORDER BY role, day`, chain, address, knownOnly)
	if err != nil {
		return Timeline{}, fmt.Errorf("timeline: %w", err)
	}
	defer rows.Close()
	tl := Timeline{Address: address, Lens: lens, Roles: map[string][]DayPoint{}}
	for rows.Next() {
		var role string
		var pt DayPoint
		if err := rows.Scan(&role, &pt.Day, &pt.TxnCount, &pt.VolumeUSDC); err != nil {
			return Timeline{}, fmt.Errorf("scan timeline: %w", err)
		}
		tl.Roles[role] = append(tl.Roles[role], pt)
	}
	if err := rows.Err(); err != nil {
		return Timeline{}, fmt.Errorf("timeline: %w", err)
	}
	if len(tl.Roles) == 0 {
		return Timeline{}, ErrNotFound
	}
	return tl, nil
}

// medianInt64 returns the lower median of an unsorted slice (0 for empty).
func medianInt64(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[(len(s)-1)/2]
}

// spanDays is the inclusive day count between two YYYY-MM-DD dates.
func spanDays(first, last string) (int64, error) {
	f, err := time.Parse("2006-01-02", first)
	if err != nil {
		return 0, fmt.Errorf("parse first day: %w", err)
	}
	l, err := time.Parse("2006-01-02", last)
	if err != nil {
		return 0, fmt.Errorf("parse last day: %w", err)
	}
	return int64(l.Sub(f).Hours()/24) + 1, nil
}

// Fingerprint implements ActivityProvider: cadence from the day series,
// price points from entity_price_point_v1, concentration from the edges.
func (p *PgEntity) Fingerprint(ctx context.Context, chain, address string, lens Lens) (Fingerprint, error) {
	tl, err := p.Timeline(ctx, chain, address, lens)
	if err != nil {
		return Fingerprint{}, err // propagates ErrNotFound
	}
	fp := Fingerprint{Address: address, Lens: lens, Roles: map[string]RoleFingerprint{}}
	for role, pts := range tl.Roles {
		rf := RoleFingerprint{ActiveDays: int64(len(pts))}
		var daily []int64
		var total, top int64
		for _, pt := range pts {
			daily = append(daily, pt.TxnCount)
			total += pt.TxnCount
			if pt.TxnCount > top {
				top = pt.TxnCount
			}
		}
		rf.MedianTxnsPerDay = medianInt64(daily)
		if rf.SpanDays, err = spanDays(pts[0].Day, pts[len(pts)-1].Day); err != nil {
			return Fingerprint{}, err
		}
		if total > 0 {
			rf.TopDayShare = fmt.Sprintf("%.6f", float64(top)/float64(total))
		} else {
			rf.TopDayShare = "0"
		}
		if err := p.fillPricePoints(ctx, chain, address, role, lens, &rf); err != nil {
			return Fingerprint{}, err
		}
		if err := p.fillConcentration(ctx, chain, address, role, lens, &rf); err != nil {
			return Fingerprint{}, err
		}
		fp.Roles[role] = rf
	}
	return fp, nil
}

// fillPricePoints merges the stored top-64 partitions for the lens. The
// distinct-amount total is only exact per stored partition, so lens=all
// leaves TotalDistinctAmounts nil rather than publish a wrong number.
func (p *PgEntity) fillPricePoints(ctx context.Context, chain, addr, role string, lens Lens, rf *RoleFingerprint) error {
	knownOnly := lens == LensKnown
	rows, err := p.pool.Query(ctx, `
		SELECT amount_usdc::text, sum(txn_count), max(total_distinct_amounts)
		FROM entity_price_point_v1
		WHERE chain = $1 AND address = $2 AND role = $3
		  AND ($4::boolean IS FALSE OR facilitator_known)
		GROUP BY amount_usdc
		ORDER BY sum(txn_count) DESC, amount_usdc
		LIMIT 16`, chain, addr, role, knownOnly)
	if err != nil {
		return fmt.Errorf("price points: %w", err)
	}
	defer rows.Close()
	var partTotal int64
	for rows.Next() {
		var pp PricePoint
		if err := rows.Scan(&pp.AmountUSDC, &pp.TxnCount, &partTotal); err != nil {
			return fmt.Errorf("scan price point: %w", err)
		}
		rf.PricePoints = append(rf.PricePoints, pp)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("price points: %w", err)
	}
	if knownOnly && partTotal > 0 {
		// total_distinct_amounts is a partition-level constant (same on every
		// row), so the last scanned value is the partition's total.
		total := partTotal
		rf.TotalDistinctAmounts = &total
	}
	return nil
}

// fillConcentration computes top-1/top-3 volume share against the direction
// total from the edge tables (payer/payee) or facilitator edges.
func (p *PgEntity) fillConcentration(ctx context.Context, chain, addr, role string, lens Lens, rf *RoleFingerprint) error {
	var cpExpr, subjectCol, table string
	switch role {
	case "payer":
		cpExpr, subjectCol, table = "payee", "payer", "entity_edge_v1"
	case "payee":
		cpExpr, subjectCol, table = "payer", "payee", "entity_edge_v1"
	case "facilitator":
		cpExpr, subjectCol, table = "counterparty", "facilitator", "facilitator_edge_v1"
	default:
		return fmt.Errorf("unknown role %q", role)
	}
	knownOnly := lens == LensKnown
	sql := fmt.Sprintf(`
		WITH agg AS (
		    SELECT %s AS cp, sum(volume_usdc) AS vol
		    FROM %s
		    WHERE chain = $1 AND %s = $2 AND ($3::boolean IS FALSE OR facilitator_known)
		    GROUP BY 1
		), ranked AS (
		    SELECT vol, row_number() OVER (ORDER BY vol DESC) AS rn FROM agg
		)
		SELECT COALESCE(round(sum(vol) FILTER (WHERE rn <= 1) / NULLIF(sum(vol), 0), 6), 0)::text,
		       COALESCE(round(sum(vol) FILTER (WHERE rn <= 3) / NULLIF(sum(vol), 0), 6), 0)::text
		FROM ranked`, cpExpr, table, subjectCol)
	if err := p.pool.QueryRow(ctx, sql, chain, addr, knownOnly).Scan(&rf.Top1Share, &rf.Top3Share); err != nil {
		return fmt.Errorf("concentration: %w", err)
	}
	return nil
}
