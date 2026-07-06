package metrics

import (
	"context"
	"errors"
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
var windowDays = map[Window]int{Window7d: 7, Window30d: 30, WindowAll: 0}

// Measure is the additive triple every roll-up returns. VolumeUSDC is a decimal
// string (never float) to preserve exactness through JSON.
type Measure struct {
	TxnCount   int64  `json:"txn_count"`
	VolumeUSDC string `json:"volume_usdc"`
}

// WindowEconomy is the economy roll-up for one window (verified/known payments
// only) plus its split by amount band.
type WindowEconomy struct {
	Measure
	ByBand map[string]Measure `json:"by_band"`
}

// DailyPoint is one day on the economy time-series chart. Complete is false for
// the newest (max) day because we cannot prove its block coverage is final —
// that day may still be accumulating payments. All earlier days are complete.
// Mirrors the MonthlyPoint.Complete convention.
type DailyPoint struct {
	Day      string `json:"day"`      // YYYY-MM-DD
	Complete bool   `json:"complete"` // false iff this is the newest (edge) day
	Measure
}

// ExcludedTotals is the all-window aggregate of payments that are NOT verified
// (cube rows with membership = 'unknown'). Emitted explicitly in economy.json
// as "excluded" so the overview panel can cite real numbers in its exclusion
// sentence. This is a deliberate, named exception to the verified-only rule:
// the block is the excluded remainder, never mixed into verified figures.
type ExcludedTotals struct {
	TxnCount   int64  `json:"txn_count"`
	VolumeUSDC string `json:"volume_usdc"`
}

// EconomyPage is the full payload for the Payment Economy page. Claims is
// attached by Emit (ResolveClaims) — BuildEconomy leaves it empty.
type EconomyPage struct {
	Windows        map[Window]WindowEconomy  `json:"windows"`
	DailySeries    []DailyPoint              `json:"daily_series"`
	MonthlySeries  []MonthlyPoint            `json:"monthly_series"`
	TypicalPayment map[Window]TypicalPayment `json:"typical_payment"`
	PricePoints    map[Window][]PricePoint   `json:"price_points"`
	Gas            GasSection                `json:"gas"`
	Velocity       VelocitySection           `json:"velocity"`
	Claims         []ClaimResult             `json:"claims"`
	Concentration  ConcentrationSection      `json:"concentration"`
	Excluded       ExcludedTotals            `json:"excluded"`
	ActiveEntities []ActiveEntitiesPoint     `json:"active_entities"`
	PayerCohorts   map[Window]PayerCohort    `json:"payer_cohorts,omitempty"`
	PriceBreadth   []PriceBreadthSeries      `json:"price_point_breadth,omitempty"`
}

// lowerBound returns the inclusive lower day (YYYY-MM-DD) for a window, or ""
// for "all" (no lower bound). "7d" means asOf's day plus the 6 preceding days
// = 7 days total, so we subtract d-1. YYYY-MM-DD strings order lexicographically,
// so day-range checks are plain string comparisons.
func lowerBound(asOf time.Time, window Window) string {
	d := windowDays[window]
	if d == 0 {
		return ""
	}
	return asOf.AddDate(0, 0, -(d - 1)).Format(dayFormat)
}

// cubeSlice is one (day, amount_band) cell of the verified (known) cube,
// bounded above by asOf. Every window, breakdown, and series point is a sum of these.
// maxAmt is the largest individual payment amount in this cell (6.2).
type cubeSlice struct {
	day    string // YYYY-MM-DD
	band   string
	txns   int64
	volume decimal.Decimal
	maxAmt decimal.Decimal // max_amount_usdc from the cube cell
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
	// Build-time consistency assertion: asOf must equal the cube's verified data
	// edge (max day with membership='known').  typical_payment and price_points
	// are anchored at rollup time to that same edge; a different asOf would make
	// those windows describe a different period than the economy series, gas, and
	// velocity sections — an internally inconsistent page with no visible error.
	var cubeMaxDay *string
	if err := q.QueryRow(ctx, `
		SELECT max(day)::text FROM metrics_daily_v2 WHERE membership = 'known'`).
		Scan(&cubeMaxDay); err != nil {
		return EconomyPage{}, fmt.Errorf("build economy: read verified data edge: %w", err)
	}
	if cubeMaxDay == nil {
		return EconomyPage{}, errors.New("build economy: cube has no verified rows — run `publisher rollup` first")
	}
	want := asOf.Format(dayFormat)
	if want != *cubeMaxDay {
		return EconomyPage{}, fmt.Errorf(
			"build economy: asOf %s does not match cube's verified data_through_day %s — "+
				"typical_payment and price_points are anchored at rollup time to %s and would be "+
				"inconsistent with a different asOf; pass the cube's own data_through_day",
			want, *cubeMaxDay, *cubeMaxDay,
		)
	}

	slices, err := readCubeSlices(ctx, q, asOf)
	if err != nil {
		return EconomyPage{}, err
	}

	page := EconomyPage{
		Windows:     map[Window]WindowEconomy{},
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
	// 6.2: inject per-window largest payment from the already-loaded slices.
	for w, ptr := range windowLargestPayments(slices, asOf) {
		tp := page.TypicalPayment[w]
		tp.LargestPaymentUSDC = ptr
		page.TypicalPayment[w] = tp
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
	if page.Excluded, err = buildExcluded(ctx, q, asOf); err != nil {
		return EconomyPage{}, err
	}
	if page.ActiveEntities, err = buildActiveEntities(ctx, q, asOf); err != nil {
		return EconomyPage{}, err
	}
	if page.PayerCohorts, err = buildPayerCohorts(ctx, q); err != nil {
		return EconomyPage{}, err
	}
	if page.PriceBreadth, err = buildPriceBreadth(ctx, q, asOf); err != nil {
		return EconomyPage{}, err
	}
	return page, nil
}

// readCubeSlices fetches the verified (known) cube cells up to and including asOf's day, in day order.
// max(max_amount_usdc) is fetched per cell for the per-window largest-payment stat (6.2).
func readCubeSlices(ctx context.Context, q Querier, asOf time.Time) ([]cubeSlice, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, amount_band, sum(txn_count), sum(volume_usdc)::text, max(max_amount_usdc)::text
		FROM metrics_daily_v2
		WHERE day <= $1::date AND membership = 'known'
		GROUP BY day, amount_band
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return nil, fmt.Errorf("economy cube read: %w", err)
	}
	defer rows.Close()

	var slices []cubeSlice
	for rows.Next() {
		var s cubeSlice
		var vol, maxAmt string
		if err := rows.Scan(&s.day, &s.band, &s.txns, &vol, &maxAmt); err != nil {
			return nil, fmt.Errorf("scan cube slice: %w", err)
		}
		if s.volume, err = decimal.NewFromString(vol); err != nil {
			return nil, fmt.Errorf("parse cube volume %q: %w", vol, err)
		}
		if s.maxAmt, err = decimal.NewFromString(maxAmt); err != nil {
			return nil, fmt.Errorf("parse cube max_amount %q: %w", maxAmt, err)
		}
		slices = append(slices, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("economy cube read: %w", err)
	}
	return slices, nil
}

// windowEconomy sums the verified slices at or after lb ("" = no lower bound)
// into one window's totals and its by-band split.
func windowEconomy(slices []cubeSlice, lb string) WindowEconomy {
	var total accum
	byBand := map[string]accum{}
	for _, s := range slices {
		if lb != "" && s.day < lb {
			continue
		}
		total = total.add(s)
		byBand[s.band] = byBand[s.band].add(s)
	}
	we := WindowEconomy{Measure: total.measure(), ByBand: map[string]Measure{}}
	for k, a := range byBand {
		we.ByBand[k] = a.measure()
	}
	return we
}

// dailySeries collapses the day-ordered slices into one point per day. It has
// no lower bound — the chart shows full history independent of the selected
// window; the UI can window it client-side. The newest (last) day is always
// marked Complete=false because we cannot prove its block coverage is final.
func dailySeries(slices []cubeSlice) []DailyPoint {
	series := []DailyPoint{}
	var cur accum
	day := ""
	flush := func() {
		if day != "" {
			series = append(series, DailyPoint{Day: day, Complete: true, Measure: cur.measure()})
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
	// The last point is always potentially partial: mark it incomplete.
	if len(series) > 0 {
		series[len(series)-1].Complete = false
	}
	return series
}

// buildExcluded queries the cube for all non-verified rows (membership = 'unknown')
// and returns their all-window totals. Returns a zero ExcludedTotals when no
// unknown rows exist (never an error in that case).
func buildExcluded(ctx context.Context, q Querier, asOf time.Time) (ExcludedTotals, error) {
	row := q.QueryRow(ctx, `
		SELECT COALESCE(sum(txn_count), 0)::bigint,
		       COALESCE(sum(volume_usdc), 0)::text
		FROM metrics_daily_v2
		WHERE day <= $1::date AND membership = 'unknown'`, asOf.Format(dayFormat))
	var ex ExcludedTotals
	var vol string
	if err := row.Scan(&ex.TxnCount, &vol); err != nil {
		return ExcludedTotals{}, fmt.Errorf("excluded totals read: %w", err)
	}
	d, err := decimal.NewFromString(vol)
	if err != nil {
		return ExcludedTotals{}, fmt.Errorf("parse excluded volume %q: %w", vol, err)
	}
	ex.VolumeUSDC = d.StringFixed(x402.USDCDecimals)
	return ex, nil
}

// FacilitatorRow is one facilitator's all-window totals plus windowed measures.
// facilitator_known is a deterministic property of the address (allowlist membership),
// not a per-row vote. Windows carries the 7d and 30d verified measures; it is always
// present in artifacts built after item 6.6 — never nil, so old-artifact readers
// must tolerate its presence.
type FacilitatorRow struct {
	Facilitator      string             `json:"facilitator"`
	FacilitatorKnown bool               `json:"facilitator_known"`
	Measure                             // all-window: txn_count and volume_usdc
	Windows          map[Window]Measure `json:"windows"` // "7d" and "30d" windowed measures
}

// FacilitatorsPage is the Facilitators page payload (top facilitators by volume).
// Totals is the sum of all rows — a self-checking block so readers can verify
// the rows conserve to the overall verified totals.
type FacilitatorsPage struct {
	Rows   []FacilitatorRow `json:"rows"`
	Totals Measure          `json:"totals"`
}

// PriceBreadthDaily is one day's payee count for one price-point amount.
// Complete mirrors the DailyPoint convention: false only for the newest day.
type PriceBreadthDaily struct {
	Day        string `json:"day"`
	PayeeCount int64  `json:"payee_count"`
	Complete   bool   `json:"complete"`
}

// PriceBreadthSeries is the daily payee-count history for one price point,
// ordered by day ascending. Series covers all history up to the asOf bound;
// the newest day's Complete is false. Absent for price points with no payments.
type PriceBreadthSeries struct {
	AmountUSDC string              `json:"amount_usdc"`
	Series     []PriceBreadthDaily `json:"series"`
}

// BuildFacilitators ranks facilitators by all-time verified volume and attaches
// 7d and 30d window measures so the UI can show momentum. The ranking and
// all-window totals are computed over all verified (known) rows up to and
// including asOf's day; the windowed measures use the same lowerBound anchoring
// convention as BuildEconomy (7d = asOf minus 6 days, inclusive; 30d = asOf
// minus 29 days, inclusive). facilitator_known is bool_or over the membership
// column — true iff the facilitator has at least one known cube row.
//
// Every allowlisted facilitator with at least one verified (known) payment
// appears — there is intentionally no row cap. Totals is computed as the
// in-Go sum of the rows so the artifact is self-checking.
func BuildFacilitators(ctx context.Context, q Querier, asOf time.Time) (FacilitatorsPage, error) {
	lb7 := lowerBound(asOf, "7d")
	lb30 := lowerBound(asOf, "30d")

	rows, err := q.Query(ctx, `
		SELECT facilitator,
		       bool_or(membership = 'known') AS facilitator_known,
		       COALESCE(sum(txn_count)   FILTER (WHERE membership = 'known'), 0)::bigint,
		       COALESCE(sum(volume_usdc) FILTER (WHERE membership = 'known'), 0)::text,
		       COALESCE(sum(txn_count)   FILTER (WHERE membership = 'known' AND day >= $2::date), 0)::bigint,
		       COALESCE(sum(volume_usdc) FILTER (WHERE membership = 'known' AND day >= $2::date), 0)::text,
		       COALESCE(sum(txn_count)   FILTER (WHERE membership = 'known' AND day >= $3::date), 0)::bigint,
		       COALESCE(sum(volume_usdc) FILTER (WHERE membership = 'known' AND day >= $3::date), 0)::text
		FROM metrics_daily_v2
		WHERE day <= $1::date
		GROUP BY facilitator
		HAVING bool_or(membership = 'known')
		ORDER BY sum(volume_usdc) FILTER (WHERE membership = 'known') DESC, facilitator`,
		asOf.Format(dayFormat), lb7, lb30)
	if err != nil {
		return FacilitatorsPage{}, fmt.Errorf("facilitators query: %w", err)
	}
	defer rows.Close()

	page := FacilitatorsPage{Rows: []FacilitatorRow{}}
	var totalTxns int64
	var totalVol decimal.Decimal
	for rows.Next() {
		var r FacilitatorRow
		var volStr, vol7Str, vol30Str string
		var txns7, txns30 int64
		if err := rows.Scan(
			&r.Facilitator, &r.FacilitatorKnown,
			&r.TxnCount, &volStr,
			&txns7, &vol7Str,
			&txns30, &vol30Str,
		); err != nil {
			return FacilitatorsPage{}, fmt.Errorf("scan facilitator row: %w", err)
		}
		vol, err := decimal.NewFromString(volStr)
		if err != nil {
			return FacilitatorsPage{}, fmt.Errorf("parse facilitator volume %q: %w", volStr, err)
		}
		r.VolumeUSDC = vol.StringFixed(x402.USDCDecimals)

		vol7, err := decimal.NewFromString(vol7Str)
		if err != nil {
			return FacilitatorsPage{}, fmt.Errorf("parse facilitator 7d volume %q: %w", vol7Str, err)
		}
		vol30, err := decimal.NewFromString(vol30Str)
		if err != nil {
			return FacilitatorsPage{}, fmt.Errorf("parse facilitator 30d volume %q: %w", vol30Str, err)
		}
		r.Windows = map[Window]Measure{
			Window7d:  {TxnCount: txns7, VolumeUSDC: vol7.StringFixed(x402.USDCDecimals)},
			Window30d: {TxnCount: txns30, VolumeUSDC: vol30.StringFixed(x402.USDCDecimals)},
		}

		totalTxns += r.TxnCount
		totalVol = totalVol.Add(vol)
		page.Rows = append(page.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return FacilitatorsPage{}, fmt.Errorf("facilitators query: %w", err)
	}
	page.Totals = Measure{
		TxnCount:   totalTxns,
		VolumeUSDC: totalVol.StringFixed(x402.USDCDecimals),
	}
	return page, nil
}

// buildPriceBreadth reads the per-day payee counts for the top-12 all-window
// price points from metrics_price_point_daily_v1, bounded above by asOf. The
// newest day is marked Complete=false (same convention as DailySeries). Returns
// an empty slice when the table has no rows (pre-rollup or pre-migration state).
func buildPriceBreadth(ctx context.Context, q Querier, asOf time.Time) ([]PriceBreadthSeries, error) {
	maxDay := asOf.Format(dayFormat)
	rows, err := q.Query(ctx, `
		SELECT pb.amount_usdc::text, pb.day::text, pb.payee_count
		FROM metrics_price_point_daily_v1 pb
		JOIN metrics_price_points_v2 pp
		    ON pp.amount_usdc = pb.amount_usdc AND pp.window_name = 'all'
		WHERE pb.day <= $1::date
		ORDER BY pp.rank, pb.day`, maxDay)
	if err != nil {
		return nil, fmt.Errorf("price breadth read: %w", err)
	}
	defer rows.Close()

	var order []string
	type seriesAccum struct{ points []PriceBreadthDaily }
	byAmount := map[string]*seriesAccum{}

	for rows.Next() {
		var amount, day string
		var count int64
		if err := rows.Scan(&amount, &day, &count); err != nil {
			return nil, fmt.Errorf("scan price breadth: %w", err)
		}
		sa, ok := byAmount[amount]
		if !ok {
			sa = &seriesAccum{}
			byAmount[amount] = sa
			order = append(order, amount)
		}
		sa.points = append(sa.points, PriceBreadthDaily{
			Day:        day,
			PayeeCount: count,
			Complete:   day != maxDay,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("price breadth read: %w", err)
	}

	result := make([]PriceBreadthSeries, 0, len(order))
	for _, amount := range order {
		result = append(result, PriceBreadthSeries{
			AmountUSDC: amount,
			Series:     byAmount[amount].points,
		})
	}
	return result, nil
}
