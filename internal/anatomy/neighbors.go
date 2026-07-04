package anatomy

import (
	"context"
	"fmt"
)

// neighborSQL aggregates one direction of edges for an address, computes each
// counterparty's share of the direction total, joins display labels, and
// returns the top-N by volume plus the honest total distinct count.
// Placeholders: %[1]s = counterparty expr, %[2]s = subject column,
// %[3]s = source table, %[4]s = extra predicate ("" or the settled-role filter).
const neighborSQL = `
WITH agg AS (
    SELECT %[1]s AS cp,
           sum(txn_count)   AS txns,
           sum(volume_usdc) AS vol,
           min(first_seen)  AS fs,
           max(last_seen)   AS ls
    FROM %[3]s
    WHERE chain = $1 AND %[2]s = $2 AND ($3::boolean IS FALSE OR facilitator_known) %[4]s
    GROUP BY %[1]s
), tot AS (
    SELECT COALESCE(sum(vol), 0) AS v, count(*) AS n FROM agg
)
SELECT a.cp, COALESCE(i.label, ''), a.txns, a.vol::text,
       CASE WHEN t.v > 0 THEN round(a.vol / t.v, 6)::text ELSE '0' END,
       a.fs::text, a.ls::text, t.n
FROM agg a
CROSS JOIN tot t
LEFT JOIN entity_identity_v1 i ON i.chain = $1 AND i.address = a.cp
ORDER BY a.vol DESC, a.cp
LIMIT $4`

// direction describes one neighbor list to fetch. All values are internal
// constants (never user input), so Sprintf into SQL is safe.
type direction struct {
	cpExpr, subjectCol, table, extra string
	assign                           func(n *Neighbors, l *NeighborList)
}

var directions = []direction{
	{
		"payee", "payer", "entity_edge_v1", "",
		func(n *Neighbors, l *NeighborList) { n.Payees = l },
	},
	{
		"payer", "payee", "entity_edge_v1", "",
		func(n *Neighbors, l *NeighborList) { n.Payers = l },
	},
	// Facilitators direction: reads by counterparty = subject with no counterparty_role
	// filter, so a subject appearing as both payer and payee gets its facilitator rows
	// merged into one list of "who settles for this address".
	{
		"facilitator", "counterparty", "facilitator_edge_v1", "",
		func(n *Neighbors, l *NeighborList) { n.Facilitators = l },
	},
	{
		"counterparty", "facilitator", "facilitator_edge_v1", "AND counterparty_role = 'payer'",
		func(n *Neighbors, l *NeighborList) { n.SettledPayers = l },
	},
	{
		"counterparty", "facilitator", "facilitator_edge_v1", "AND counterparty_role = 'payee'",
		func(n *Neighbors, l *NeighborList) { n.SettledPayees = l },
	},
}

// Neighbors implements NeighborProvider.
func (p *PgEntity) Neighbors(ctx context.Context, chain, address, lens string, limit int) (Neighbors, error) {
	n := Neighbors{Address: address, Lens: lens}
	knownOnly := lens == "known"
	found := false
	for _, d := range directions {
		sql := fmt.Sprintf(neighborSQL, d.cpExpr, d.subjectCol, d.table, d.extra)
		rows, err := p.pool.Query(ctx, sql, chain, address, knownOnly, limit)
		if err != nil {
			return Neighbors{}, fmt.Errorf("neighbors %s: %w", d.cpExpr, err)
		}
		list := &NeighborList{}
		for rows.Next() {
			var r NeighborRow
			if err := rows.Scan(&r.Address, &r.Label, &r.TxnCount, &r.VolumeUSDC,
				&r.Share, &r.FirstSeen, &r.LastSeen, &list.Total); err != nil {
				rows.Close()
				return Neighbors{}, fmt.Errorf("scan neighbor: %w", err)
			}
			list.Rows = append(list.Rows, r)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return Neighbors{}, fmt.Errorf("neighbors %s: %w", d.cpExpr, err)
		}
		rows.Close()
		if len(list.Rows) > 0 {
			d.assign(&n, list)
			found = true
		}
	}
	if !found {
		return Neighbors{}, ErrNotFound
	}
	return n, nil
}
