package metrics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/lukostrobl/fathom/internal/x402"
)

// MonthlyPoint is one month on the economy trend chart (E5). Complete is false
// when the month is cut by either data edge, so the UI never derives MoM
// growth from a partial month.
type MonthlyPoint struct {
	Month    string `json:"month"` // YYYY-MM
	Complete bool   `json:"complete"`
	Measure
	ByMembership map[string]Measure `json:"by_membership"`
}

// TypicalPayment answers "what does a typical payment look like" (E7).
type TypicalPayment struct {
	AvgUSDC    string `json:"avg_usdc"`
	MedianUSDC string `json:"median_usdc"`
	TxnCount   int64  `json:"txn_count"`
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

// GasWindow splits one window's gas by membership and by amount band.
type GasWindow struct {
	ByMembership map[string]GasMeasure `json:"by_membership"`
	ByBand       map[string]GasMeasure `json:"by_band"`
}

// GasSection carries the gas windows plus the methodology notes that make the
// number citable.
type GasSection struct {
	Method  map[string]string    `json:"method"`
	Windows map[string]GasWindow `json:"windows"`
}

// VelocityStat is the burstiness headline for one window × membership (E12).
type VelocityStat struct {
	MaxPerMin int64 `json:"max_per_min"`
}

// VelocityPoint is one day of the burstiness series.
type VelocityPoint struct {
	Day        string `json:"day"`
	Membership string `json:"membership"`
	MaxPerMin  int64  `json:"max_per_min"`
	P99PerMin  int64  `json:"p99_per_min"`
}

// VelocitySection is the E12 payload.
type VelocitySection struct {
	Windows     map[string]map[string]VelocityStat `json:"windows"`
	DailySeries []VelocityPoint                    `json:"daily_series"`
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
	"price":       "monthly ETH/USD reference prices from data/eth-usd-monthly.json (source cited in the file)",
	"breakeven":   "payments whose apportioned gas in USD exceeds the amount moved",
	"granularity": "rolled up from (day, membership, amount_band)",
	"cost":        "true L2 settlement cost = execution gas + L1 data fee",
}

// monthlySeries collapses day-ordered cube slices into calendar months. A
// month is complete iff its first day >= the earliest data day AND its last
// day <= asOf's day. The series is sparse — a calendar month with zero payments
// is simply absent, like the daily series.
func monthlySeries(slices []cubeSlice, asOf time.Time) ([]MonthlyPoint, error) {
	if len(slices) == 0 {
		return []MonthlyPoint{}, nil
	}
	minDay := slices[0].day
	asOfDay := asOf.Format(dayFormat)

	type monthAccum struct {
		total    accum
		byMember map[string]accum
	}
	order := []string{}
	months := map[string]*monthAccum{}
	for _, s := range slices {
		m := s.day[:7]
		ma, ok := months[m]
		if !ok {
			ma = &monthAccum{byMember: map[string]accum{}}
			months[m] = ma
			order = append(order, m)
		}
		ma.total = ma.total.add(s)
		ma.byMember[s.membership] = ma.byMember[s.membership].add(s)
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
			Month:        m,
			Complete:     monthStart >= minDay && monthEnd <= asOfDay,
			Measure:      months[m].total.measure(),
			ByMembership: map[string]Measure{},
		}
		for k, a := range months[m].byMember {
			mp.ByMembership[k] = a.measure()
		}
		series = append(series, mp)
	}
	return series, nil
}

// buildTypicalPayment merges cube-side averages with rollup-side medians.
// windows is the already-built map from BuildEconomy.
func buildTypicalPayment(ctx context.Context, q Querier, windows map[string]WindowEconomy) (map[string]map[string]TypicalPayment, error) {
	out := map[string]map[string]TypicalPayment{}
	for w := range windowDays {
		out[w] = map[string]TypicalPayment{}
	}

	rows, err := q.Query(ctx, `
		SELECT window_name, membership, txn_count, median_amount_usdc::text
		FROM metrics_window_stats_v2`)
	if err != nil {
		return nil, fmt.Errorf("window stats read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var w, member, median string
		var txns int64
		if err := rows.Scan(&w, &member, &txns, &median); err != nil {
			return nil, fmt.Errorf("scan window stats: %w", err)
		}
		if _, ok := out[w]; !ok {
			return nil, fmt.Errorf("window stats read: unknown window_name %q", w)
		}
		var m Measure
		if member == "all" {
			m = windows[w].Measure
		} else {
			m = windows[w].ByMembership[member]
		}
		out[w][member] = TypicalPayment{
			AvgUSDC:    avgUSDC(m),
			MedianUSDC: median,
			TxnCount:   txns,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("window stats read: %w", err)
	}
	return out, nil
}

// avgUSDC divides a Measure's volume by its count, "0.000000" for empty.
func avgUSDC(m Measure) string {
	if m.TxnCount == 0 {
		return decimal.Zero.StringFixed(x402.USDCDecimals)
	}
	vol, err := decimal.NewFromString(m.VolumeUSDC)
	if err != nil {
		return decimal.Zero.StringFixed(x402.USDCDecimals)
	}
	return vol.Div(decimal.NewFromInt(m.TxnCount)).StringFixed(x402.USDCDecimals)
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
		knownTxns := windows[w].ByMembership["known"].TxnCount
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

// gasSlice mirrors one metrics_gas_daily_v2 row with exact decimals.
type gasSlice struct {
	day        string
	membership string
	band       string
	txns       int64
	l2         decimal.Decimal
	l1         decimal.Decimal
	usd        decimal.Decimal
	breakeven  int64
	volume     decimal.Decimal
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

// buildGas windows the daily gas table by membership and band.
func buildGas(ctx context.Context, q Querier, asOf time.Time) (GasSection, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, membership, amount_band, txn_count,
		       l2_gas_cost_wei::text, l1_fee_wei::text, cost_usd::text, breakeven_txn_count, volume_usdc::text
		FROM metrics_gas_daily_v2
		WHERE day <= $1::date
		ORDER BY day`, asOf.Format(dayFormat))
	if err != nil {
		return GasSection{}, fmt.Errorf("gas read: %w", err)
	}
	defer rows.Close()

	var slices []gasSlice
	for rows.Next() {
		var s gasSlice
		var l2, l1, usd, vol string
		if err := rows.Scan(&s.day, &s.membership, &s.band, &s.txns, &l2, &l1, &usd, &s.breakeven, &vol); err != nil {
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
		byMember := map[string]gasAccum{}
		byBand := map[string]gasAccum{}
		for _, s := range slices {
			if lb != "" && s.day < lb {
				continue
			}
			byMember[s.membership] = byMember[s.membership].add(s)
			byBand[s.band] = byBand[s.band].add(s)
		}
		gw := GasWindow{ByMembership: map[string]GasMeasure{}, ByBand: map[string]GasMeasure{}}
		for k, a := range byMember {
			gw.ByMembership[k] = a.measure()
		}
		for k, a := range byBand {
			gw.ByBand[k] = a.measure()
		}
		sec.Windows[w] = gw
	}
	return sec, nil
}

// buildVelocity windows the daily velocity table and carries the full series.
func buildVelocity(ctx context.Context, q Querier, asOf time.Time) (VelocitySection, error) {
	rows, err := q.Query(ctx, `
		SELECT day::text, membership, max_per_min, p99_per_min
		FROM metrics_velocity_daily_v2
		WHERE day <= $1::date
		ORDER BY day, membership`, asOf.Format(dayFormat))
	if err != nil {
		return VelocitySection{}, fmt.Errorf("velocity read: %w", err)
	}
	defer rows.Close()

	sec := VelocitySection{Windows: map[string]map[string]VelocityStat{}, DailySeries: []VelocityPoint{}}
	for rows.Next() {
		var p VelocityPoint
		if err := rows.Scan(&p.Day, &p.Membership, &p.MaxPerMin, &p.P99PerMin); err != nil {
			return VelocitySection{}, fmt.Errorf("scan velocity point: %w", err)
		}
		sec.DailySeries = append(sec.DailySeries, p)
	}
	if err := rows.Err(); err != nil {
		return VelocitySection{}, fmt.Errorf("velocity read: %w", err)
	}

	for w := range windowDays {
		lb := lowerBound(asOf, w)
		stats := map[string]VelocityStat{}
		for _, p := range sec.DailySeries {
			if lb != "" && p.Day < lb {
				continue
			}
			if p.MaxPerMin > stats[p.Membership].MaxPerMin {
				stats[p.Membership] = VelocityStat{MaxPerMin: p.MaxPerMin}
			}
		}
		sec.Windows[w] = stats
	}
	return sec, nil
}

// ResolveClaims joins the curated ledger to measured numbers from the built
// page. Pure function — no DB — so the claim ledger can be re-emitted without
// a rescan, and unit tests need no container.
func ResolveClaims(page EconomyPage, claims []Claim) ([]ClaimResult, error) {
	out := make([]ClaimResult, 0, len(claims))
	for _, c := range claims {
		subject, kind, window, err := parseMetric(c.MeasuredMetric)
		if err != nil {
			return nil, fmt.Errorf("claim %s: %w", c.ID, err)
		}
		var m Measure
		if subject == "total" {
			m = page.Windows[window].Measure
		} else {
			m = page.Windows[window].ByMembership[subject] // zero Measure if absent
		}
		r := ClaimResult{Claim: c}
		switch kind {
		case "txns":
			r.MeasuredValue = strconv.FormatInt(m.TxnCount, 10)
			r.MeasuredUnit = "transactions"
		case "volume":
			r.MeasuredValue = m.VolumeUSDC
			if r.MeasuredValue == "" {
				r.MeasuredValue = decimal.Zero.StringFixed(x402.USDCDecimals)
			}
			r.MeasuredUnit = "USDC"
		default:
			return nil, fmt.Errorf("claim %s: unhandled kind %q", c.ID, kind)
		}
		out = append(out, r)
	}
	return out, nil
}
