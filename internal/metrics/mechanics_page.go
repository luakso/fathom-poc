package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// FeeBlock is the M4 EIP-1559 fee-intent summary for one slice.
type FeeBlock struct {
	TxType      map[string]int64 `json:"tx_type"`
	MaxFee      Percentiles      `json:"max_fee"`
	MaxPriority Percentiles      `json:"max_priority"`
}

// Percentiles holds nullable p50/p90/p99 (string-decimals for the NUMERIC fee
// columns; nil when the slice had no rows).
type Percentiles struct {
	P50 *string `json:"p50"`
	P90 *string `json:"p90"`
	P99 *string `json:"p99"`
}

// WidthBlock is the M7 authorization-window-width summary (seconds).
type WidthBlock struct {
	Count int64    `json:"count"`
	P50S  *float64 `json:"p50_s"`
	P90S  *float64 `json:"p90_s"`
	P99S  *float64 `json:"p99_s"`
}

// OverProvBlock is the M9 gas over-provisioning summary (gas_used/gas_limit ratio).
type OverProvBlock struct {
	Count    int64    `json:"count"`
	RatioP50 *float64 `json:"ratio_p50"`
	RatioP90 *float64 `json:"ratio_p90"`
	RatioP99 *float64 `json:"ratio_p99"`
}

// HygieneBlock is the M12 data-hygiene canary.
type HygieneBlock struct {
	DupAuthNonce    int64 `json:"dup_auth_nonce"`
	SameBlockReplay int64 `json:"same_block_replay"`
}

// BatchBucket is one M3 batch-size histogram bar.
type BatchBucket struct {
	Bucket       string `json:"bucket"`
	TxCount      int64  `json:"tx_count"`
	PaymentCount int64  `json:"payment_count"`
}

// BatchBlock is the M3 batch summary.
type BatchBlock struct {
	Histogram    []BatchBucket `json:"histogram"`
	PctBatched   float64       `json:"pct_batched"`
	MaxBatchSize int64         `json:"max_batch_size"`
}

// BlockDensity is the M5 payments-per-block summary.
type BlockDensity struct {
	MaxPerBlock    int64   `json:"max_per_block"`
	P99PerBlock    int64   `json:"p99_per_block"`
	MeanPerBlock   float64 `json:"mean_per_block"`
	DistinctBlocks int64   `json:"distinct_blocks"`
}

// SelectorRow is one M2 wrapper/selector-mix entry.
type SelectorRow struct {
	SelectorHex    string `json:"selector_hex"`
	SettlementKind string `json:"settlement_kind"`
	TxnCount       int64  `json:"txn_count"`
	VolumeUSDC     string `json:"volume_usdc"`
}

// MechanicsMeasure is the per-(window, membership) per-payment slice.
type MechanicsMeasure struct {
	SettlementCount  int64         `json:"settlement_count"`
	Fee              FeeBlock      `json:"fee"`
	AuthWindowWidth  WidthBlock    `json:"auth_window_width"`
	OverProvisioning OverProvBlock `json:"over_provisioning"`
	TxValueNonzero   int64         `json:"tx_value_nonzero"`
	Hygiene          HygieneBlock  `json:"hygiene"`
	SelectorMix      []SelectorRow `json:"selector_mix"`
}

// MechanicsWindow adds the window-grain blocks (batch, block density, cost) to
// the verified ('known') measure.
type MechanicsWindow struct {
	MechanicsMeasure
	Batch        BatchBlock   `json:"batch"`
	BlockDensity BlockDensity `json:"block_density"`
	Cost         GasMeasure   `json:"cost"`
}

// MechanicsPage is the mechanics.json payload.
type MechanicsPage struct {
	Windows map[string]MechanicsWindow `json:"windows"`
}

// BuildMechanics assembles mechanics.json from the four mechanics tables plus the
// reused gas cube (metrics_gas_daily_v2) for the cost headline.
func BuildMechanics(ctx context.Context, q Querier) (MechanicsPage, error) {
	page := MechanicsPage{Windows: map[string]MechanicsWindow{}}
	for w := range windowDays {
		page.Windows[w] = MechanicsWindow{
			MechanicsMeasure: MechanicsMeasure{Fee: FeeBlock{TxType: map[string]int64{}}, SelectorMix: []SelectorRow{}},
		}
	}

	if err := readMechWindow(ctx, q, page); err != nil {
		return MechanicsPage{}, err
	}
	if err := readMechSelector(ctx, q, page); err != nil {
		return MechanicsPage{}, err
	}
	if err := readMechBatch(ctx, q, page); err != nil {
		return MechanicsPage{}, err
	}
	if err := readMechBlock(ctx, q, page); err != nil {
		return MechanicsPage{}, err
	}
	if err := readMechCost(ctx, q, page); err != nil {
		return MechanicsPage{}, err
	}
	return page, nil
}

func readMechWindow(ctx context.Context, q Querier, page MechanicsPage) error {
	rows, err := q.Query(ctx, `
		SELECT window_name, membership, settlement_count,
		       tx_type_0_count, tx_type_1_count, tx_type_2_count,
		       max_fee_p50::text, max_fee_p90::text, max_fee_p99::text,
		       max_priority_p50::text, max_priority_p90::text, max_priority_p99::text,
		       width_count, width_p50_s, width_p90_s, width_p99_s,
		       overprov_count, overprov_ratio_p50, overprov_ratio_p90, overprov_ratio_p99,
		       tx_value_nonzero_count, dup_auth_nonce_count, same_block_replay_count
		FROM metrics_mechanics_window_v2`)
	if err != nil {
		return fmt.Errorf("mechanics window read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wname, membership string
		var m MechanicsMeasure
		var t0, t1, t2 int64
		var mf50, mf90, mf99, mp50, mp90, mp99 *string
		if err := rows.Scan(&wname, &membership, &m.SettlementCount,
			&t0, &t1, &t2,
			&mf50, &mf90, &mf99, &mp50, &mp90, &mp99,
			&m.AuthWindowWidth.Count, &m.AuthWindowWidth.P50S, &m.AuthWindowWidth.P90S, &m.AuthWindowWidth.P99S,
			&m.OverProvisioning.Count, &m.OverProvisioning.RatioP50, &m.OverProvisioning.RatioP90, &m.OverProvisioning.RatioP99,
			&m.TxValueNonzero, &m.Hygiene.DupAuthNonce, &m.Hygiene.SameBlockReplay); err != nil {
			return fmt.Errorf("scan mechanics window: %w", err)
		}
		m.Fee = FeeBlock{
			TxType:      map[string]int64{"0": t0, "1": t1, "2": t2},
			MaxFee:      Percentiles{P50: mf50, P90: mf90, P99: mf99},
			MaxPriority: Percentiles{P50: mp50, P90: mp90, P99: mp99},
		}
		m.SelectorMix = []SelectorRow{}
		win, ok := page.Windows[wname]
		if !ok {
			return fmt.Errorf("mechanics: unknown window %q", wname)
		}
		if membership == "known" {
			win.MechanicsMeasure = m
		}
		page.Windows[wname] = win
	}
	return rows.Err()
}

func readMechSelector(ctx context.Context, q Querier, page MechanicsPage) error {
	rows, err := q.Query(ctx, `
		SELECT window_name, membership, selector_hex, settlement_kind, txn_count, volume_usdc::text
		FROM metrics_mechanics_selector_v2 ORDER BY window_name, membership, rank`)
	if err != nil {
		return fmt.Errorf("mechanics selector read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wname, membership string
		var r SelectorRow
		if err := rows.Scan(&wname, &membership, &r.SelectorHex, &r.SettlementKind, &r.TxnCount, &r.VolumeUSDC); err != nil {
			return fmt.Errorf("scan mechanics selector: %w", err)
		}
		win, ok := page.Windows[wname]
		if !ok {
			return fmt.Errorf("mechanics selector: unknown window %q", wname)
		}
		if membership == "known" {
			win.SelectorMix = append(win.SelectorMix, r)
		}
		page.Windows[wname] = win
	}
	return rows.Err()
}

func readMechBatch(ctx context.Context, q Querier, page MechanicsPage) error {
	rows, err := q.Query(ctx, `
		SELECT window_name, batch_bucket, tx_count, payment_count, max_batch_size
		FROM metrics_mechanics_batch_v2 ORDER BY window_name, batch_bucket`)
	if err != nil {
		return fmt.Errorf("mechanics batch read: %w", err)
	}
	defer rows.Close()
	totals := map[string]int64{}
	batched := map[string]int64{}
	for rows.Next() {
		var wname, bucket string
		var txc, payc, maxb int64
		if err := rows.Scan(&wname, &bucket, &txc, &payc, &maxb); err != nil {
			return fmt.Errorf("scan mechanics batch: %w", err)
		}
		win, ok := page.Windows[wname]
		if !ok {
			return fmt.Errorf("mechanics batch: unknown window %q", wname)
		}
		win.Batch.Histogram = append(win.Batch.Histogram, BatchBucket{Bucket: bucket, TxCount: txc, PaymentCount: payc})
		win.Batch.MaxBatchSize = maxb
		page.Windows[wname] = win
		totals[wname] += payc
		if bucket != "1" {
			batched[wname] += payc
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for wname, win := range page.Windows {
		if totals[wname] > 0 {
			win.Batch.PctBatched = float64(batched[wname]) / float64(totals[wname])
			page.Windows[wname] = win
		}
	}
	return nil
}

func readMechBlock(ctx context.Context, q Querier, page MechanicsPage) error {
	rows, err := q.Query(ctx, `
		SELECT window_name, max_per_block, p99_per_block, mean_per_block, distinct_blocks
		FROM metrics_mechanics_block_v2`)
	if err != nil {
		return fmt.Errorf("mechanics block read: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var wname string
		var bd BlockDensity
		if err := rows.Scan(&wname, &bd.MaxPerBlock, &bd.P99PerBlock, &bd.MeanPerBlock, &bd.DistinctBlocks); err != nil {
			return fmt.Errorf("scan mechanics block: %w", err)
		}
		win, ok := page.Windows[wname]
		if !ok {
			return fmt.Errorf("mechanics block: unknown window %q", wname)
		}
		win.BlockDensity = bd
		page.Windows[wname] = win
	}
	return rows.Err()
}

// readMechCost rolls metrics_gas_daily_v2 into a per-window cost headline, reusing
// economy.go's gasSlice/gasAccum. Windows are anchored to the cube's max day, the
// same anchoring the rollup uses.
func readMechCost(ctx context.Context, q Querier, page MechanicsPage) error {
	var maxDay *string
	if err := q.QueryRow(ctx, `SELECT max(day)::text FROM metrics_gas_daily_v2`).Scan(&maxDay); err != nil {
		return fmt.Errorf("mechanics cost anchor: %w", err)
	}
	if maxDay == nil {
		return nil // no gas data; cost stays zero-value
	}
	asOf, err := time.Parse(dayFormat, *maxDay)
	if err != nil {
		return fmt.Errorf("parse cost anchor %q: %w", *maxDay, err)
	}
	rows, err := q.Query(ctx, `
		SELECT day::text, l2_gas_cost_wei::text, l1_fee_wei::text, cost_usd::text, breakeven_txn_count, txn_count, volume_usdc::text
		FROM metrics_gas_daily_v2 WHERE membership = 'known' AND day <= $1::date`, asOf.Format(dayFormat))
	if err != nil {
		return fmt.Errorf("mechanics cost read: %w", err)
	}
	defer rows.Close()
	type dayCost struct {
		day              time.Time
		l2, l1, usd, vol decimal.Decimal
		breakeven, txns  int64
	}
	var costs []dayCost
	for rows.Next() {
		var d, l2, l1, usd, vol string
		var be, tx int64
		if err := rows.Scan(&d, &l2, &l1, &usd, &be, &tx, &vol); err != nil {
			return fmt.Errorf("scan mechanics cost: %w", err)
		}
		day, perr := time.Parse(dayFormat, d)
		if perr != nil {
			return fmt.Errorf("parse cost day %q: %w", d, perr)
		}
		dc := dayCost{day: day, breakeven: be, txns: tx}
		if dc.l2, err = decimal.NewFromString(l2); err != nil {
			return fmt.Errorf("parse cost l2 %q: %w", l2, err)
		}
		if dc.l1, err = decimal.NewFromString(l1); err != nil {
			return fmt.Errorf("parse cost l1 %q: %w", l1, err)
		}
		if dc.usd, err = decimal.NewFromString(usd); err != nil {
			return fmt.Errorf("parse cost usd %q: %w", usd, err)
		}
		if dc.vol, err = decimal.NewFromString(vol); err != nil {
			return fmt.Errorf("parse cost vol %q: %w", vol, err)
		}
		costs = append(costs, dc)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for w, days := range windowDays {
		var acc gasAccum
		for _, c := range costs {
			if days == 0 || !c.day.Before(asOf.AddDate(0, 0, -(days-1))) {
				acc = acc.add(gasSlice{txns: c.txns, l2: c.l2, l1: c.l1, usd: c.usd, breakeven: c.breakeven, volume: c.vol})
			}
		}
		win := page.Windows[w]
		win.Cost = acc.measure()
		page.Windows[w] = win
	}
	return nil
}
