package anatomy

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// errBadCursor is returned by parseCursor when the cursor string is malformed
// or contains values that overflow int64.
var errBadCursor = errors.New("malformed cursor")

// cpSortCols whitelists ORDER BY expressions for the counterparty table.
// Keys are the public sort names; values are safe SQL (never user input).
var cpSortCols = map[string]string{
	"volume":    "vol DESC",
	"txns":      "txns DESC",
	"last_seen": "ls DESC",
}

// Counterparties implements ListProvider: the paginated inspector table.
// Reuses neighborSQL's aggregation shape with OFFSET pagination and a
// selectable sort.
func (p *PgEntity) Counterparties(ctx context.Context, chain, address string, q CounterpartyQuery) (CounterpartyPage, error) {
	order, ok := cpSortCols[q.Sort]
	if !ok {
		order = cpSortCols["volume"]
	}
	var cpExpr, subjectCol, table string
	switch q.Role {
	case "payer":
		cpExpr, subjectCol, table = "payee", "payer", "entity_edge_v1"
	case "payee":
		cpExpr, subjectCol, table = "payer", "payee", "entity_edge_v1"
	case "facilitator":
		cpExpr, subjectCol, table = "counterparty", "facilitator", "facilitator_edge_v1"
	default:
		return CounterpartyPage{}, fmt.Errorf("unknown role %q", q.Role)
	}
	knownOnly := q.Lens == "known"
	//nolint:gosec // G201: cpExpr/subjectCol/table/order are internal constants, never user input
	sql := fmt.Sprintf(`
		WITH agg AS (
		    SELECT %s AS cp, sum(txn_count) AS txns, sum(volume_usdc) AS vol,
		           min(first_seen) AS fs, max(last_seen) AS ls
		    FROM %s
		    WHERE chain = $1 AND %s = $2 AND ($3::boolean IS FALSE OR facilitator_known)
		    GROUP BY 1
		), tot AS (SELECT COALESCE(sum(vol),0) AS v, count(*) AS n FROM agg)
		SELECT a.cp, COALESCE(i.label,''), a.txns, a.vol::text,
		       CASE WHEN t.v > 0 THEN round(a.vol/t.v, 6)::text ELSE '0' END,
		       a.fs::text, a.ls::text, t.n
		FROM agg a CROSS JOIN tot t
		LEFT JOIN entity_identity_v1 i ON i.chain = $1 AND i.address = a.cp
		ORDER BY %s, a.cp
		LIMIT $4 OFFSET $5`, cpExpr, table, subjectCol, order)
	rows, err := p.pool.Query(ctx, sql, chain, address, knownOnly, q.Limit, q.Offset)
	if err != nil {
		return CounterpartyPage{}, fmt.Errorf("counterparties: %w", err)
	}
	defer rows.Close()
	page := CounterpartyPage{Address: address, Role: q.Role, Lens: q.Lens}
	for rows.Next() {
		var r NeighborRow
		if err := rows.Scan(&r.Address, &r.Label, &r.TxnCount, &r.VolumeUSDC,
			&r.Share, &r.FirstSeen, &r.LastSeen, &page.Total); err != nil {
			return CounterpartyPage{}, fmt.Errorf("scan counterparty: %w", err)
		}
		page.Rows = append(page.Rows, r)
	}
	return page, rows.Err()
}

// paymentRoleCols whitelists the payments filter column per subject role.
var paymentRoleCols = map[string]string{
	"payer": "payer", "payee": "payee", "facilitator": "facilitator",
}

// Payments implements ListProvider: the ONLY live query against payments.
// Keyset-paginated on (block_number, tx_hash, log_index) DESC. Note: the
// table has single-column address indexes, so Postgres top-N sorts the
// matched rows; mega-facilitator subjects can hit the request timeout —
// a documented v1 limitation (composite indexes deferred for disk headroom).
//
// The cursor predicate uses the expanded OR form rather than the row-constructor
// form `(a,b,c) < ($5,$6,$7)` to avoid pgx type-inference failures on mixed
// (bigint, text, int) tuples.
func (p *PgEntity) Payments(ctx context.Context, chain, address string, q PaymentQuery) (PaymentPage, error) {
	col, ok := paymentRoleCols[q.Role]
	if !ok {
		return PaymentPage{}, fmt.Errorf("unknown role %q", q.Role)
	}
	cursorBlock, cursorHash, cursorIdx, useCursor, err := parseCursor(q.Before)
	if err != nil {
		return PaymentPage{}, err
	}
	knownOnly := q.Lens == "known"
	//nolint:gosec // G201: col is an internal whitelist constant, never user input
	sql := fmt.Sprintf(`
		SELECT tx_hash, log_index, block_number, block_timestamp::text,
		       payer, payee, facilitator, amount_usdc::text, facilitator_known
		FROM payment_x402_v1
		WHERE chain = $1 AND %s = $2
		  AND ($3::boolean IS FALSE OR facilitator_known)
		  AND ($4::boolean IS FALSE OR (
		      block_number < $5 OR
		      (block_number = $5 AND (tx_hash < $6 OR (tx_hash = $6 AND log_index < $7)))
		  ))
		ORDER BY block_number DESC, tx_hash DESC, log_index DESC
		LIMIT $8`, col)
	rows, err := p.pool.Query(ctx, sql, chain, address, knownOnly,
		useCursor, cursorBlock, cursorHash, cursorIdx, q.Limit+1)
	if err != nil {
		return PaymentPage{}, fmt.Errorf("payments: %w", err)
	}
	defer rows.Close()
	page := PaymentPage{Address: address, Role: q.Role, Lens: q.Lens}
	for rows.Next() {
		var r PaymentRow
		if err := rows.Scan(&r.TxHash, &r.LogIndex, &r.BlockNumber, &r.BlockTimestamp,
			&r.Payer, &r.Payee, &r.Facilitator, &r.AmountUSDC, &r.FacilitatorKnown); err != nil {
			return PaymentPage{}, fmt.Errorf("scan payment: %w", err)
		}
		page.Rows = append(page.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return PaymentPage{}, fmt.Errorf("payments: %w", err)
	}
	if len(page.Rows) > q.Limit {
		page.Rows = page.Rows[:q.Limit]
		last := page.Rows[len(page.Rows)-1]
		page.Next = fmt.Sprintf("%d:%s:%d", last.BlockNumber, last.TxHash, last.LogIndex)
	}
	return page, nil
}

// parseCursor decodes "blockNumber:txHash:logIndex". Empty cursor = first page.
// Returns errBadCursor for any malformed or overflowing input.
func parseCursor(s string) (block int64, hash string, idx int64, use bool, err error) {
	if s == "" {
		return 0, "", 0, false, nil
	}
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0, "", 0, false, errBadCursor
	}
	if block, err = strconv.ParseInt(parts[0], 10, 64); err != nil {
		return 0, "", 0, false, errBadCursor
	}
	if idx, err = strconv.ParseInt(parts[2], 10, 64); err != nil {
		return 0, "", 0, false, errBadCursor
	}
	return block, parts[1], idx, true, nil
}
