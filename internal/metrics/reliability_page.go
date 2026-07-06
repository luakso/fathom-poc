package metrics

import (
	"context"
	"fmt"
)

// LatencyBuckets is the fixed-shape sign→settle latency histogram. The JSON keys
// reproduce the prior open-map shape exactly.
type LatencyBuckets struct {
	Sub1s   int64 `json:"sub1s"`
	B1To10s int64 `json:"1_10s"`
	B10To60 int64 `json:"10_60s"`
	B1To10m int64 `json:"1_10m"`
	GT10m   int64 `json:"gt10m"`
}

// ReliabilityLatency is the R2 sign→settle distribution for one slice. Percentiles
// are nil when the slice has no windowed settlements.
type ReliabilityLatency struct {
	P50S    *float64       `json:"p50_s"`
	P90S    *float64       `json:"p90_s"`
	P99S    *float64       `json:"p99_s"`
	Buckets LatencyBuckets `json:"buckets"`
}

// ReliabilityMeasure is the reliability summary for one (window, membership).
// Rates are derived Go-side from the counts; denominators are guarded to 0.
type ReliabilityMeasure struct {
	SettlementCount   int64              `json:"settlement_count"`
	WindowedCount     int64              `json:"windowed_count"`
	WindowedShare     float64            `json:"windowed_share"`
	CancellationCount int64              `json:"cancellation_count"`
	CancellationRate  float64            `json:"cancellation_rate"`
	Latency           ReliabilityLatency `json:"latency"`
	ExpiredCount      int64              `json:"expired_count"`
	ExpiredRate       float64            `json:"expired_rate"`
	NotYetValidCount  int64              `json:"not_yet_valid_count"`
	NotYetValidRate   float64            `json:"not_yet_valid_rate"`
}

// ReliabilityWindow is one window: the verified (known) totals inline.
type ReliabilityWindow struct {
	ReliabilityMeasure
}

// ReliabilityDailyPoint is one day of the R1/R3/R4 trend (both memberships summed).
type ReliabilityDailyPoint struct {
	Day               string `json:"day"`
	SettlementCount   int64  `json:"settlement_count"`
	WindowedCount     int64  `json:"windowed_count"`
	ExpiredCount      int64  `json:"expired_count"`
	NotYetValidCount  int64  `json:"not_yet_valid_count"`
	CancellationCount int64  `json:"cancellation_count"`
}

// CancellationActor is one row of the R6 attribution leaderboard.
type CancellationActor struct {
	Address          string `json:"address"`
	Count            int64  `json:"count"`
	FacilitatorKnown bool   `json:"facilitator_known"`
}

// ReliabilityPage is the reliability.json payload.
type ReliabilityPage struct {
	Windows                 map[Window]ReliabilityWindow `json:"windows"`
	Daily                   []ReliabilityDailyPoint      `json:"daily"`
	CancellationAttribution struct {
		ByPayer      []CancellationActor `json:"by_payer"`
		ByCancelFrom []CancellationActor `json:"by_cancel_from"`
	} `json:"cancellation_attribution"`
}

// rate divides safely; a zero denominator yields 0 (mirrors the site's pct() guard).
func rate(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return float64(num) / float64(den)
}

// BuildReliability assembles reliability.json from the two rollup tables plus the
// cancellation view (R6, read live — 33 rows in production).
func BuildReliability(ctx context.Context, q Querier) (ReliabilityPage, error) {
	page := ReliabilityPage{Windows: map[Window]ReliabilityWindow{}}
	for w := range windowDays {
		page.Windows[w] = ReliabilityWindow{}
	}

	wrows, err := q.Query(ctx, `
		SELECT window_name, membership, settlement_count, windowed_count, cancellation_count,
		       latency_p50_s, latency_p90_s, latency_p99_s,
		       lat_bucket_sub1s, lat_bucket_1_10s, lat_bucket_10_60s, lat_bucket_1_10m, lat_bucket_gt10m,
		       expired_count, not_yet_valid_count
		FROM metrics_reliability_window_v2`)
	if err != nil {
		return ReliabilityPage{}, fmt.Errorf("reliability window read: %w", err)
	}
	defer wrows.Close()
	for wrows.Next() {
		var wname, membership string
		var m ReliabilityMeasure
		var b struct{ sub1, b110s, b1060s, b110m, gt10m int64 }
		if err := wrows.Scan(&wname, &membership, &m.SettlementCount, &m.WindowedCount, &m.CancellationCount,
			&m.Latency.P50S, &m.Latency.P90S, &m.Latency.P99S,
			&b.sub1, &b.b110s, &b.b1060s, &b.b110m, &b.gt10m,
			&m.ExpiredCount, &m.NotYetValidCount); err != nil {
			return ReliabilityPage{}, fmt.Errorf("scan reliability window: %w", err)
		}
		m.WindowedShare = rate(m.WindowedCount, m.SettlementCount)
		m.CancellationRate = rate(m.CancellationCount, m.SettlementCount)
		m.ExpiredRate = rate(m.ExpiredCount, m.WindowedCount)
		m.NotYetValidRate = rate(m.NotYetValidCount, m.WindowedCount)
		m.Latency.Buckets = LatencyBuckets{
			Sub1s: b.sub1, B1To10s: b.b110s, B10To60: b.b1060s, B1To10m: b.b110m, GT10m: b.gt10m,
		}
		win, ok := page.Windows[Window(wname)]
		if !ok {
			return ReliabilityPage{}, fmt.Errorf("reliability: unknown window %q", wname)
		}
		if Membership(membership) == MembershipKnown {
			win.ReliabilityMeasure = m
		}
		page.Windows[Window(wname)] = win
	}
	if err := wrows.Err(); err != nil {
		return ReliabilityPage{}, fmt.Errorf("reliability window read: %w", err)
	}

	page.Daily, err = readReliabilityDaily(ctx, q)
	if err != nil {
		return ReliabilityPage{}, err
	}
	if err := readCancellationAttribution(ctx, q, &page); err != nil {
		return ReliabilityPage{}, err
	}
	return page, nil
}

// readReliabilityDaily sums both memberships per day, ascending.
func readReliabilityDaily(ctx context.Context, q Querier) ([]ReliabilityDailyPoint, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text,
		       sum(settlement_count), sum(windowed_count),
		       sum(expired_count), sum(not_yet_valid_count), sum(cancellation_count)
		FROM metrics_reliability_daily_v2
		WHERE membership = 'known'
		GROUP BY day ORDER BY day`)
	if err != nil {
		return nil, fmt.Errorf("reliability daily read: %w", err)
	}
	defer rows.Close()
	out := []ReliabilityDailyPoint{}
	for rows.Next() {
		var p ReliabilityDailyPoint
		if err := rows.Scan(&p.Day, &p.SettlementCount, &p.WindowedCount,
			&p.ExpiredCount, &p.NotYetValidCount, &p.CancellationCount); err != nil {
			return nil, fmt.Errorf("scan reliability daily: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// readCancellationAttribution fills R6: top payers (authorizer) and cancel
// submitters (transaction_from) by cancellation count, from the live view.
func readCancellationAttribution(ctx context.Context, q Querier, page *ReliabilityPage) error {
	page.CancellationAttribution.ByPayer = []CancellationActor{}
	page.CancellationAttribution.ByCancelFrom = []CancellationActor{}
	for _, spec := range []struct {
		col  string
		dest *[]CancellationActor
	}{
		{"authorizer", &page.CancellationAttribution.ByPayer},
		{"transaction_from", &page.CancellationAttribution.ByCancelFrom},
	} {
		// col is a fixed internal constant, never user input — safe to interpolate.
		rows, err := q.Query(ctx, fmt.Sprintf(`
			SELECT %[1]s, count(*), bool_or(facilitator_known)
			FROM authorization_cancellation_v1
			WHERE facilitator_known
			GROUP BY %[1]s ORDER BY count(*) DESC, %[1]s LIMIT 100`, spec.col))
		if err != nil {
			return fmt.Errorf("cancellation attribution %s: %w", spec.col, err)
		}
		for rows.Next() {
			var a CancellationActor
			if err := rows.Scan(&a.Address, &a.Count, &a.FacilitatorKnown); err != nil {
				rows.Close()
				return fmt.Errorf("scan cancellation attribution: %w", err)
			}
			*spec.dest = append(*spec.dest, a)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("cancellation attribution %s: %w", spec.col, err)
		}
	}
	return nil
}
