package metrics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/lukostrobl/fathom/internal/x402"
)

// MonthlyPoint is one month on the economy trend chart (E5), verified payments
// only. Complete is false when the month is cut by either data edge, so the UI
// never derives MoM growth from a partial month.
type MonthlyPoint struct {
	Month    string `json:"month"` // YYYY-MM
	Complete bool   `json:"complete"`
	Measure
}

// TypicalPayment answers "what does a typical payment look like" (E7).
// P10/P90/P99 are omitempty: they are absent in pre-6.3 artifacts (until the
// next rollup+emit after migration 00019). JS checks for nil before showing
// the percentile strip.
type TypicalPayment struct {
	AvgUSDC    string  `json:"avg_usdc"`
	MedianUSDC string  `json:"median_usdc"`
	TxnCount   int64   `json:"txn_count"`
	P10USDC    *string `json:"p10_usdc,omitempty"`
	P90USDC    *string `json:"p90_usdc,omitempty"`
	P99USDC    *string `json:"p99_usdc,omitempty"`
}

// PricePoint is one row of the agentic price-point spectrum (E8).
type PricePoint struct {
	AmountUSDC  string `json:"amount_usdc"`
	TxnCount    int64  `json:"txn_count"`
	VolumeUSDC  string `json:"volume_usdc"`
	PayeeCount  int64  `json:"payee_count"`
	TxnSharePct string `json:"txn_share_pct"` // share of known-facilitator txns in the window
}

// GasMeasure is the settlement-overhead roll-up for one split (E11).
type GasMeasure struct {
	TxnCount          int64   `json:"txn_count"`
	GasETH            string  `json:"gas_eth"`              // total (l1+l2)
	GasETHL1          string  `json:"gas_eth_l1"`           // L1 data-fee component
	GasETHL2          string  `json:"gas_eth_l2"`           // L2 execution component
	GasUSD            string  `json:"gas_usd"`              // total cost in USD
	GasCentsPerDollar *string `json:"gas_cents_per_dollar"` // null when the split moved $0
	BreakevenTxnCount int64   `json:"breakeven_txn_count"`
}

// GasWindow holds the verified gas headline for one window plus a by-band split.
type GasWindow struct {
	GasMeasure
	ByBand map[string]GasMeasure `json:"by_band"`
}

// GasSection carries the gas windows plus the methodology notes that make the
// number citable.
type GasSection struct {
	Method  map[string]string    `json:"method"`
	Windows map[string]GasWindow `json:"windows"`
}

// ActiveEntitiesPoint is one day of the active-wallet daily series (6.1).
// Payers and payees are counted distinctly over verified payments only;
// the same wallet on two days is counted once per day, not globally merged.
// Complete is false for the newest (edge) day — the same convention as DailyPoint.
type ActiveEntitiesPoint struct {
	Day        string `json:"day"`      // YYYY-MM-DD
	Complete   bool   `json:"complete"` // false iff this is the newest (edge) day
	PayerCount int64  `json:"payer_count"`
	PayeeCount int64  `json:"payee_count"`
}

// VelocityStat is the burstiness headline for one window (E12).
type VelocityStat struct {
	MaxPerMin int64 `json:"max_per_min"`
}

// VelocityPoint is one day of the burstiness series (verified payments only).
type VelocityPoint struct {
	Day       string `json:"day"`
	MaxPerMin int64  `json:"max_per_min"`
	P99PerMin int64  `json:"p99_per_min"`
}

// VelocitySection is the E12 payload (verified payments only).
type VelocitySection struct {
	Windows     map[string]VelocityStat `json:"windows"`
	DailySeries []VelocityPoint         `json:"daily_series"`
}

// ClaimResult is a curated claim joined to its measured counterpart (E6).
type ClaimResult struct {
	Claim
	MeasuredValue string `json:"measured_value"`
	MeasuredUnit  string `json:"measured_unit"`
}

// gasMethodNotes documents how the gas numbers were computed; emitted verbatim.
var gasMethodNotes = map[string]string{
	"dedupe":      "both the L2 execution gas and the L1 data fee are tx-level; each is counted once per transaction and apportioned equally across its payments",
	"price":       "weekly ETH/USD reference prices from data/eth-usd-weekly.json (ISO Monday week-start; source cited in the file)",
	"breakeven":   "payments whose apportioned gas in USD exceeds the amount moved",
	"granularity": "rolled up from verified (known-facilitator) (day, amount_band) gas rows",
	"cost":        "true L2 settlement cost = execution gas + L1 data fee",
}

// monthlySeries collapses day-ordered verified cube slices into calendar months.
// A month is complete iff its first day >= the earliest data day AND its last
// day <= asOf's day. The series is sparse — a calendar month with zero verified
// payments is simply absent, like the daily series.
func monthlySeries(slices []cubeSlice, asOf time.Time) ([]MonthlyPoint, error) {
	if len(slices) == 0 {
		return []MonthlyPoint{}, nil
	}
	minDay := slices[0].day
	asOfDay := asOf.Format(dayFormat)

	type monthAccum struct {
		total accum
	}
	order := []string{}
	months := map[string]*monthAccum{}
	for _, s := range slices {
		m := s.day[:7]
		ma, ok := months[m]
		if !ok {
			ma = &monthAccum{}
			months[m] = ma
			order = append(order, m)
		}
		ma.total = ma.total.add(s)
	}

	series := make([]MonthlyPoint, 0, len(order))
	for _, m := range order {
		first, err := time.Parse("2006-01", m)
		if err != nil {
			return nil, fmt.Errorf("parse month %q: %w", m, err)
		}
		monthStart := first.Format(dayFormat)
		monthEnd := first.AddDate(0, 1, -1).Format(dayFormat)
		mp := MonthlyPoint{
			Month:    m,
			Complete: monthStart >= minDay && monthEnd <= asOfDay,
			Measure:  months[m].total.measure(),
		}
		series = append(series, mp)
	}
	return series, nil
}

// buildTypicalPayment merges cube-side averages with rollup-side medians for
// verified payments only. windows is the already-built map from BuildEconomy.
// Every window is pre-initialised so the output always contains all three keys
// even when a window has no verified rows in metrics_window_stats_v2.
func buildTypicalPayment(ctx context.Context, q Querier, windows map[string]WindowEconomy) (map[string]TypicalPayment, error) {
	zero := decimal.Zero.StringFixed(x402.USDCDecimals)
	out := map[string]TypicalPayment{}
	for w := range windowDays {
		out[w] = TypicalPayment{AvgUSDC: zero, MedianUSDC: zero}
	}
	rows, err := q.Query(ctx, `
		SELECT window_name, txn_count, median_amount_usdc::text,
		       p10_amount_usdc::text, p90_amount_usdc::text, p99_amount_usdc::text
		FROM metrics_window_stats_v2 WHERE membership = 'known'`)
	if err != nil {
		return nil, fmt.Errorf("window stats read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w, median string
		var txns int64
		var p10, p90, p99 *string
		if err := rows.Scan(&w, &txns, &median, &p10, &p90, &p99); err != nil {
			return nil, fmt.Errorf("scan window stats: %w", err)
		}
		if _, ok := windows[w]; !ok {
			return nil, fmt.Errorf("window stats read: unknown window_name %q", w)
		}
		avg, err := avgUSDC(windows[w].Measure)
		if err != nil {
			return nil, fmt.Errorf("avg for window %s: %w", w, err)
		}
		out[w] = TypicalPayment{
			AvgUSDC:    avg,
			MedianUSDC: median,
			TxnCount:   txns,
			P10USDC:    p10,
			P90USDC:    p90,
			P99USDC:    p99,
		}
	}
	return out, rows.Err()
}

// avgUSDC divides a Measure's volume by its count. Returns "0.000000" for an
// empty measure (TxnCount == 0).  Returns an error if VolumeUSDC is not a
// valid decimal — callers must not swallow this into a silent zero.
func avgUSDC(m Measure) (string, error) {
	if m.TxnCount == 0 {
		return decimal.Zero.StringFixed(x402.USDCDecimals), nil
	}
	vol, err := decimal.NewFromString(m.VolumeUSDC)
	if err != nil {
		return "", fmt.Errorf("avgUSDC: parse volume %q: %w", m.VolumeUSDC, err)
	}
	return vol.Div(decimal.NewFromInt(m.TxnCount)).StringFixed(x402.USDCDecimals), nil
}

// buildPricePoints reads the precomputed top-N and attaches the window share.
func buildPricePoints(ctx context.Context, q Querier, windows map[string]WindowEconomy) (map[string][]PricePoint, error) {
	out := map[string][]PricePoint{}
	for w := range windowDays {
		out[w] = []PricePoint{}
	}

	rows, err := q.Query(ctx, `
		SELECT window_name, amount_usdc::text, txn_count, volume_usdc::text, payee_count
		FROM metrics_price_points_v2
		ORDER BY window_name, rank`)
	if err != nil {
		return nil, fmt.Errorf("price points read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w string
		var p PricePoint
		if err := rows.Scan(&w, &p.AmountUSDC, &p.TxnCount, &p.VolumeUSDC, &p.PayeeCount); err != nil {
			return nil, fmt.Errorf("scan price point: %w", err)
		}
		if _, ok := out[w]; !ok {
			return nil, fmt.Errorf("price points read: unknown window_name %q", w)
		}
		knownTxns := windows[w].TxnCount
		share := decimal.Zero
		if knownTxns > 0 {
			share = decimal.NewFromInt(p.TxnCount).
				Mul(decimal.NewFromInt(100)).
				Div(decimal.NewFromInt(knownTxns))
		}
		p.TxnSharePct = share.StringFixed(2)
		out[w] = append(out[w], p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("price points read: %w", err)
	}
	return out, nil
}

// gasSlice mirrors one verified metrics_gas_daily_v2 row with exact decimals.
type gasSlice struct {
	day       string
	band      string
	txns      int64
	l2        decimal.Decimal
	l1        decimal.Decimal
	usd       decimal.Decimal
	breakeven int64
	volume    decimal.Decimal
}

// gasAccum accumulates gasSlices for one split.
type gasAccum struct {
	txns, breakeven  int64
	l2, l1, usd, vol decimal.Decimal
}

func (a gasAccum) add(s gasSlice) gasAccum {
	return gasAccum{
		txns: a.txns + s.txns, breakeven: a.breakeven + s.breakeven,
		l2: a.l2.Add(s.l2), l1: a.l1.Add(s.l1), usd: a.usd.Add(s.usd), vol: a.vol.Add(s.volume),
	}
}

func (a gasAccum) measure() GasMeasure {
	gm := GasMeasure{
		TxnCount:          a.txns,
		GasETH:            a.l2.Add(a.l1).Shift(-18).StringFixed(6),
		GasETHL1:          a.l1.Shift(-18).StringFixed(6),
		GasETHL2:          a.l2.Shift(-18).StringFixed(6),
		GasUSD:            a.usd.StringFixed(2),
		BreakevenTxnCount: a.breakeven,
	}
	if a.vol.IsPositive() {
		cents := a.usd.Mul(decimal.NewFromInt(100)).Div(a.vol).StringFixed(4)
		gm.GasCentsPerDollar = &cents
	}
	return gm
}

// buildGas windows the daily gas table for verified payments by band.
func buildGas(ctx context.Context, q Querier, asOf time.Time) (GasSection, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, amount_band, txn_count,
		       l2_gas_cost_wei::text, l1_fee_wei::text, cost_usd::text, breakeven_txn_count, volume_usdc::text
		FROM metrics_gas_daily_v2
		WHERE day <= $1::date AND membership = 'known'
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return GasSection{}, fmt.Errorf("gas read: %w", err)
	}
	defer rows.Close()

	var slices []gasSlice
	for rows.Next() {
		var s gasSlice
		var l2, l1, usd, vol string
		if err := rows.Scan(&s.day, &s.band, &s.txns, &l2, &l1, &usd, &s.breakeven, &vol); err != nil {
			return GasSection{}, fmt.Errorf("scan gas slice: %w", err)
		}
		if s.l2, err = decimal.NewFromString(l2); err != nil {
			return GasSection{}, fmt.Errorf("parse gas l2 %q: %w", l2, err)
		}
		if s.l1, err = decimal.NewFromString(l1); err != nil {
			return GasSection{}, fmt.Errorf("parse gas l1 %q: %w", l1, err)
		}
		if s.usd, err = decimal.NewFromString(usd); err != nil {
			return GasSection{}, fmt.Errorf("parse gas usd %q: %w", usd, err)
		}
		if s.volume, err = decimal.NewFromString(vol); err != nil {
			return GasSection{}, fmt.Errorf("parse gas volume %q: %w", vol, err)
		}
		slices = append(slices, s)
	}
	if err := rows.Err(); err != nil {
		return GasSection{}, fmt.Errorf("gas read: %w", err)
	}

	sec := GasSection{Method: gasMethodNotes, Windows: map[string]GasWindow{}}
	for w := range windowDays {
		lb := lowerBound(asOf, w)
		var total gasAccum
		byBand := map[string]gasAccum{}
		for _, s := range slices {
			if lb != "" && s.day < lb {
				continue
			}
			total = total.add(s)
			byBand[s.band] = byBand[s.band].add(s)
		}
		gw := GasWindow{GasMeasure: total.measure(), ByBand: map[string]GasMeasure{}}
		for k, a := range byBand {
			gw.ByBand[k] = a.measure()
		}
		sec.Windows[w] = gw
	}
	return sec, nil
}

// buildVelocity windows the daily velocity table for verified payments and carries the full series.
func buildVelocity(ctx context.Context, q Querier, asOf time.Time) (VelocitySection, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, max_per_min, p99_per_min
		FROM metrics_velocity_daily_v2
		WHERE day <= $1::date AND membership = 'known'
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return VelocitySection{}, fmt.Errorf("velocity read: %w", err)
	}
	defer rows.Close()

	sec := VelocitySection{Windows: map[string]VelocityStat{}, DailySeries: []VelocityPoint{}}
	for rows.Next() {
		var p VelocityPoint
		if err := rows.Scan(&p.Day, &p.MaxPerMin, &p.P99PerMin); err != nil {
			return VelocitySection{}, fmt.Errorf("scan velocity point: %w", err)
		}
		sec.DailySeries = append(sec.DailySeries, p)
	}
	if err := rows.Err(); err != nil {
		return VelocitySection{}, fmt.Errorf("velocity read: %w", err)
	}

	for w := range windowDays {
		lb := lowerBound(asOf, w)
		var stat VelocityStat
		for _, p := range sec.DailySeries {
			if lb != "" && p.Day < lb {
				continue
			}
			if p.MaxPerMin > stat.MaxPerMin {
				stat = VelocityStat{MaxPerMin: p.MaxPerMin}
			}
		}
		sec.Windows[w] = stat
	}
	return sec, nil
}

// ResolveClaims joins the curated ledger to measured numbers from the built
// page. All claims resolve against the verified (known) window total. Pure
// function — no DB — so the claim ledger can be re-emitted without a rescan,
// and unit tests need no container.
func ResolveClaims(page EconomyPage, claims []Claim) ([]ClaimResult, error) {
	out := make([]ClaimResult, 0, len(claims))
	for _, c := range claims {
		_, kind, window, err := parseMetric(c.MeasuredMetric)
		if err != nil {
			return nil, fmt.Errorf("claim %s: %w", c.ID, err)
		}
		we, ok := page.Windows[window]
		if !ok {
			return nil, fmt.Errorf("claim %s: window %q not present in economy page — rebuild and re-emit", c.ID, window)
		}
		m := we.Measure
		r := ClaimResult{Claim: c}
		switch kind {
		case "txns":
			r.MeasuredValue = strconv.FormatInt(m.TxnCount, 10)
			r.MeasuredUnit = "transactions"
		case "volume":
			// VolumeUSDC from accum.measure() is always a valid decimal string;
			// no empty-string fallback needed after the ok-check above.
			r.MeasuredValue = m.VolumeUSDC
			r.MeasuredUnit = "USDC"
		default:
			return nil, fmt.Errorf("claim %s: unhandled kind %q", c.ID, kind)
		}
		out = append(out, r)
	}
	return out, nil
}

// buildActiveEntities reads metrics_active_entities_daily_v2 up to and including
// asOf's day and marks the newest (edge) day Complete=false — the same convention
// as the daily economy series.
func buildActiveEntities(ctx context.Context, q Querier, asOf time.Time) ([]ActiveEntitiesPoint, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, payer_count, payee_count
		FROM metrics_active_entities_daily_v2
		WHERE day <= $1::date
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return nil, fmt.Errorf("active entities read: %w", err)
	}
	defer rows.Close()
	series := []ActiveEntitiesPoint{}
	for rows.Next() {
		var p ActiveEntitiesPoint
		p.Complete = true
		if err := rows.Scan(&p.Day, &p.PayerCount, &p.PayeeCount); err != nil {
			return nil, fmt.Errorf("scan active entities: %w", err)
		}
		series = append(series, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("active entities read: %w", err)
	}
	// The newest day is always potentially partial: its block window may still
	// be accumulating payments.
	if len(series) > 0 {
		series[len(series)-1].Complete = false
	}
	return series, nil
}
