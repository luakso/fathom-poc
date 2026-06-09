package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// windowDays maps a window name to its lookback in days. "all" (0) has no lower bound.
var windowDays = map[string]int{"7d": 7, "30d": 30, "all": 0}

// Measure is the additive triple every roll-up returns. VolumeUSDC is a decimal
// string (never float) to preserve exactness through JSON.
type Measure struct {
	TxnCount   int64  `json:"txn_count"`
	VolumeUSDC string `json:"volume_usdc"`
}

// WindowEconomy is the economy roll-up for one window, plus its split by
// attribution and by amount band.
type WindowEconomy struct {
	Measure
	ByAttribution map[string]Measure `json:"by_attribution"`
	ByBand        map[string]Measure `json:"by_band"`
}

// DailyPoint is one day on the economy time-series chart.
type DailyPoint struct {
	Day string `json:"day"` // YYYY-MM-DD
	Measure
}

// EconomyPage is the full payload for the Payment Economy page.
type EconomyPage struct {
	Windows     map[string]WindowEconomy `json:"windows"`
	DailySeries []DailyPoint             `json:"daily_series"`
}

// lowerBound returns the inclusive lower timestamp for a window, or zero time for "all".
// "7d" means asOf's day plus the 6 preceding days = 7 days total, so we subtract d-1.
func lowerBound(asOf time.Time, window string) time.Time {
	d := windowDays[window]
	if d == 0 {
		return time.Time{}
	}
	return asOf.AddDate(0, 0, -(d - 1))
}

// BuildEconomy rolls the cube up into the economy page. asOf pins "now" so
// windows are deterministic (pass time.Now().UTC() in production).
func BuildEconomy(ctx context.Context, pool *pgxpool.Pool, asOf time.Time) (EconomyPage, error) {
	page := EconomyPage{Windows: map[string]WindowEconomy{}, DailySeries: []DailyPoint{}}

	for window := range windowDays {
		lb := lowerBound(asOf, window)
		we := WindowEconomy{ByAttribution: map[string]Measure{}, ByBand: map[string]Measure{}}

		if err := pool.QueryRow(ctx, `
			SELECT coalesce(sum(txn_count),0), coalesce(sum(volume_usdc), 0::numeric(38,6))::text
			FROM metrics_daily_v1
			WHERE ($1::date IS NULL OR day >= $1::date)`,
			nullableDate(lb)).Scan(&we.TxnCount, &we.VolumeUSDC); err != nil {
			return EconomyPage{}, fmt.Errorf("economy totals %s: %w", window, err)
		}
		if err := fillBreakdown(ctx, pool, lb, "attribution", we.ByAttribution); err != nil {
			return EconomyPage{}, fmt.Errorf("economy by_attribution %s: %w", window, err)
		}
		if err := fillBreakdown(ctx, pool, lb, "amount_band", we.ByBand); err != nil {
			return EconomyPage{}, fmt.Errorf("economy by_band %s: %w", window, err)
		}
		page.Windows[window] = we
	}

	// Daily series is deliberately unbounded — the chart shows full history
	// independent of the selected window; the UI can window it client-side.
	rows, err := pool.Query(ctx, `
		SELECT day::text, sum(txn_count), sum(volume_usdc)::text
		FROM metrics_daily_v1 GROUP BY day ORDER BY day`)
	if err != nil {
		return EconomyPage{}, fmt.Errorf("economy daily series: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p DailyPoint
		if err := rows.Scan(&p.Day, &p.TxnCount, &p.VolumeUSDC); err != nil {
			return EconomyPage{}, fmt.Errorf("scan daily point: %w", err)
		}
		page.DailySeries = append(page.DailySeries, p)
	}
	return page, rows.Err()
}

// fillBreakdown groups the cube by one column within a window into dst.
func fillBreakdown(ctx context.Context, pool *pgxpool.Pool, lb time.Time, col string, dst map[string]Measure) error {
	switch col {
	case "attribution", "amount_band":
	default:
		return fmt.Errorf("fillBreakdown: unknown column %q", col)
	}
	// col is a fixed internal value ('attribution' | 'amount_band'), never user input.
	q := fmt.Sprintf(`
		SELECT %s, sum(txn_count), sum(volume_usdc)::text
		FROM metrics_daily_v1
		WHERE ($1::date IS NULL OR day >= $1::date)
		GROUP BY %s`, col, col)
	rows, err := pool.Query(ctx, q, nullableDate(lb))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var m Measure
		if err := rows.Scan(&key, &m.TxnCount, &m.VolumeUSDC); err != nil {
			return err
		}
		dst[key] = m
	}
	return rows.Err()
}

// FacilitatorRow is one facilitator's 'all'-window totals.
type FacilitatorRow struct {
	Facilitator string `json:"facilitator"`
	Attribution string `json:"attribution"`
	Measure
}

// FacilitatorsPage is the Facilitators page payload (top facilitators by volume).
type FacilitatorsPage struct {
	Rows []FacilitatorRow `json:"rows"`
}

// BuildFacilitators ranks facilitators by all-time volume. asOf is accepted for
// signature symmetry with BuildEconomy / future windowing; the ranking is
// all-window for Phase 1a.
func BuildFacilitators(ctx context.Context, pool *pgxpool.Pool, asOf time.Time) (FacilitatorsPage, error) {
	rows, err := pool.Query(ctx, `
		SELECT facilitator,
		       (array_agg(attribution ORDER BY att_volume DESC))[1] AS attribution,
		       sum(att_txn_count),
		       sum(att_volume)::text
		FROM (
		    SELECT facilitator, attribution,
		           sum(volume_usdc) AS att_volume,
		           sum(txn_count)   AS att_txn_count
		    FROM metrics_daily_v1
		    GROUP BY facilitator, attribution
		) sub
		GROUP BY facilitator
		ORDER BY sum(att_volume) DESC
		LIMIT 100`)
	if err != nil {
		return FacilitatorsPage{}, fmt.Errorf("facilitators query: %w", err)
	}
	defer rows.Close()

	page := FacilitatorsPage{Rows: []FacilitatorRow{}}
	for rows.Next() {
		var r FacilitatorRow
		if err := rows.Scan(&r.Facilitator, &r.Attribution, &r.TxnCount, &r.VolumeUSDC); err != nil {
			return FacilitatorsPage{}, fmt.Errorf("scan facilitator row: %w", err)
		}
		page.Rows = append(page.Rows, r)
	}
	return page, rows.Err()
}

// nullableDate returns nil for the zero time (→ SQL NULL, meaning "no lower
// bound" / all history) and a YYYY-MM-DD string otherwise. A string (not a
// time.Time) is used so `$1::date` is a clean text→date cast with no server
// timezone in play.
func nullableDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format("2006-01-02")
}
