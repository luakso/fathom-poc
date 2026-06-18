package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/lukostrobl/fathom/internal/x402"
)

// dayFormat is the YYYY-MM-DD layout the cube's day column round-trips through.
const dayFormat = "2006-01-02"

// Querier is the read surface the page builders need. Both *pgxpool.Pool and
// pgx.Tx satisfy it; Emit passes a REPEATABLE READ transaction so every page
// is built from one consistent cube snapshot.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// windowDays maps a window name to its lookback in days. "all" (0) has no lower bound.
var windowDays = map[string]int{"7d": 7, "30d": 30, "all": 0}

// Measure is the additive triple every roll-up returns. VolumeUSDC is a decimal
// string (never float) to preserve exactness through JSON.
type Measure struct {
	TxnCount   int64  `json:"txn_count"`
	VolumeUSDC string `json:"volume_usdc"`
}

// WindowEconomy is the economy roll-up for one window, plus its split by
// membership and by amount band.
type WindowEconomy struct {
	Measure
	ByMembership map[string]Measure `json:"by_membership"`
	ByBand       map[string]Measure `json:"by_band"`
}

// DailyPoint is one day on the economy time-series chart.
type DailyPoint struct {
	Day string `json:"day"` // YYYY-MM-DD
	Measure
}

// EconomyPage is the full payload for the Payment Economy page. Claims is
// attached by Emit (ResolveClaims) — BuildEconomy leaves it empty.
type EconomyPage struct {
	Windows        map[string]WindowEconomy             `json:"windows"`
	DailySeries    []DailyPoint                         `json:"daily_series"`
	MonthlySeries  []MonthlyPoint                       `json:"monthly_series"`
	TypicalPayment map[string]map[string]TypicalPayment `json:"typical_payment"`
	PricePoints    map[string][]PricePoint              `json:"price_points"`
	Gas            GasSection                           `json:"gas"`
	Velocity       VelocitySection                      `json:"velocity"`
	Claims         []ClaimResult                        `json:"claims"`
}

// lowerBound returns the inclusive lower day (YYYY-MM-DD) for a window, or ""
// for "all" (no lower bound). "7d" means asOf's day plus the 6 preceding days
// = 7 days total, so we subtract d-1. YYYY-MM-DD strings order lexicographically,
// so day-range checks are plain string comparisons.
func lowerBound(asOf time.Time, window string) string {
	d := windowDays[window]
	if d == 0 {
		return ""
	}
	return asOf.AddDate(0, 0, -(d - 1)).Format(dayFormat)
}

// cubeSlice is one (day, membership, amount_band) cell of the cube, bounded
// above by asOf. Every window, breakdown, and series point is a sum of these.
type cubeSlice struct {
	day        string // YYYY-MM-DD
	membership string
	band       string
	txns       int64
	volume     decimal.Decimal
}

// accum is a Measure under construction: integer count plus exact decimal sum,
// formatted to the cube's scale exactly once at the end.
type accum struct {
	txns int64
	vol  decimal.Decimal
}

func (a accum) add(s cubeSlice) accum {
	return accum{txns: a.txns + s.txns, vol: a.vol.Add(s.volume)}
}

func (a accum) measure() Measure {
	return Measure{TxnCount: a.txns, VolumeUSDC: a.vol.StringFixed(x402.USDCDecimals)}
}

// BuildEconomy rolls the cube up into the economy page. asOf pins "now" so
// windows are deterministic: every window (including "all" and the daily
// series) is bounded above by asOf's day, and "7d"/"30d" reach back from it.
// Emit passes the cube's own data_through_day, so windows always end at the
// data's edge regardless of when the artifacts are regenerated.
//
// One query reads the day × membership × amount_band slices; windows,
// breakdowns, and the daily series are Go-side sums over them. shopspring
// decimals keep the math exact — the cube's NUMERIC(38,6) text round-trips
// losslessly.
func BuildEconomy(ctx context.Context, q Querier, asOf time.Time) (EconomyPage, error) {
	slices, err := readCubeSlices(ctx, q, asOf)
	if err != nil {
		return EconomyPage{}, err
	}

	page := EconomyPage{
		Windows:     map[string]WindowEconomy{},
		DailySeries: dailySeries(slices),
		Claims:      []ClaimResult{},
	}
	for window := range windowDays {
		page.Windows[window] = windowEconomy(slices, lowerBound(asOf, window))
	}

	if page.MonthlySeries, err = monthlySeries(slices, asOf); err != nil {
		return EconomyPage{}, err
	}
	if page.TypicalPayment, err = buildTypicalPayment(ctx, q, page.Windows); err != nil {
		return EconomyPage{}, err
	}
	if page.PricePoints, err = buildPricePoints(ctx, q, page.Windows); err != nil {
		return EconomyPage{}, err
	}
	if page.Gas, err = buildGas(ctx, q, asOf); err != nil {
		return EconomyPage{}, err
	}
	if page.Velocity, err = buildVelocity(ctx, q, asOf); err != nil {
		return EconomyPage{}, err
	}
	return page, nil
}

// readCubeSlices fetches the cube cells up to and including asOf's day, in day order.
func readCubeSlices(ctx context.Context, q Querier, asOf time.Time) ([]cubeSlice, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, membership, amount_band, sum(txn_count), sum(volume_usdc)::text
		FROM metrics_daily_v2
		WHERE day <= $1::date
		GROUP BY day, membership, amount_band
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return nil, fmt.Errorf("economy cube read: %w", err)
	}
	defer rows.Close()

	var slices []cubeSlice
	for rows.Next() {
		var s cubeSlice
		var vol string
		if err := rows.Scan(&s.day, &s.membership, &s.band, &s.txns, &vol); err != nil {
			return nil, fmt.Errorf("scan cube slice: %w", err)
		}
		if s.volume, err = decimal.NewFromString(vol); err != nil {
			return nil, fmt.Errorf("parse cube volume %q: %w", vol, err)
		}
		slices = append(slices, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("economy cube read: %w", err)
	}
	return slices, nil
}

// windowEconomy sums the slices at or after lb ("" = no lower bound) into one
// window's totals and its by-membership / by-band splits.
func windowEconomy(slices []cubeSlice, lb string) WindowEconomy {
	var total accum
	byMember := map[string]accum{}
	byBand := map[string]accum{}
	for _, s := range slices {
		if lb != "" && s.day < lb {
			continue
		}
		total = total.add(s)
		byMember[s.membership] = byMember[s.membership].add(s)
		byBand[s.band] = byBand[s.band].add(s)
	}

	we := WindowEconomy{Measure: total.measure(), ByMembership: map[string]Measure{}, ByBand: map[string]Measure{}}
	for k, a := range byMember {
		we.ByMembership[k] = a.measure()
	}
	for k, a := range byBand {
		we.ByBand[k] = a.measure()
	}
	return we
}

// dailySeries collapses the day-ordered slices into one point per day. It has
// no lower bound — the chart shows full history independent of the selected
// window; the UI can window it client-side.
func dailySeries(slices []cubeSlice) []DailyPoint {
	series := []DailyPoint{}
	var cur accum
	day := ""
	flush := func() {
		if day != "" {
			series = append(series, DailyPoint{Day: day, Measure: cur.measure()})
		}
	}
	for _, s := range slices {
		if s.day != day {
			flush()
			day, cur = s.day, accum{}
		}
		cur = cur.add(s)
	}
	flush()
	return series
}

// FacilitatorRow is one facilitator's all-window totals. facilitator_known is a
// deterministic property of the address (allowlist membership), not a per-row vote.
type FacilitatorRow struct {
	Facilitator      string `json:"facilitator"`
	FacilitatorKnown bool   `json:"facilitator_known"`
	Measure
}

// FacilitatorsPage is the Facilitators page payload (top facilitators by volume).
type FacilitatorsPage struct {
	Rows []FacilitatorRow `json:"rows"`
}

// BuildFacilitators ranks facilitators by all-time volume. The ranking is
// all-window for Phase 1a; windowed rankings can add an asOf parameter when
// they exist. facilitator_known is a deterministic allowlist property —
// bool_or over the membership column is true iff any of the facilitator's
// cube cells are 'known'.
func BuildFacilitators(ctx context.Context, q Querier) (FacilitatorsPage, error) {
	rows, err := q.Query(ctx, `
		SELECT facilitator,
		       bool_or(membership = 'known') AS facilitator_known,
		       sum(txn_count),
		       sum(volume_usdc)::text
		FROM metrics_daily_v2
		GROUP BY facilitator
		ORDER BY sum(volume_usdc) DESC, facilitator
		LIMIT 100`)
	if err != nil {
		return FacilitatorsPage{}, fmt.Errorf("facilitators query: %w", err)
	}
	defer rows.Close()

	page := FacilitatorsPage{Rows: []FacilitatorRow{}}
	for rows.Next() {
		var r FacilitatorRow
		if err := rows.Scan(&r.Facilitator, &r.FacilitatorKnown, &r.TxnCount, &r.VolumeUSDC); err != nil {
			return FacilitatorsPage{}, fmt.Errorf("scan facilitator row: %w", err)
		}
		page.Rows = append(page.Rows, r)
	}
	return page, rows.Err()
}
