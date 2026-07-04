// Package anatomy assembles a single-transaction dossier graph and per-address
// statistics from the payments substrate. It is read-only.
package anatomy

import "errors"

// ErrNotFound is returned when a transaction or address has no rows.
var ErrNotFound = errors.New("anatomy: not found")

// NodeKind enumerates the node types in a dossier graph.
type NodeKind string

const (
	// NodeTransaction is a payment transaction node.
	NodeTransaction NodeKind = "transaction"
	// NodeEvent is an event node.
	NodeEvent NodeKind = "event"
	// NodeAddress is an address node.
	NodeAddress NodeKind = "address"
)

// Role is a part an address plays in a payment.
type Role string

const (
	// RolePayer indicates the address is a payer.
	RolePayer Role = "payer"
	// RolePayee indicates the address is a payee.
	RolePayee Role = "payee"
	// RoleFacilitator indicates the address is a facilitator.
	RoleFacilitator Role = "facilitator"
)

// ProviderRef declares an expandable data provider on a node. Available=false
// renders as a disabled "coming soon" stub.
type ProviderRef struct {
	Kind      string `json:"kind"`
	Available bool   `json:"available"`
}

// Node is a vertex in the dossier graph.
type Node struct {
	ID        string            `json:"id"`
	Kind      NodeKind          `json:"kind"`
	Label     string            `json:"label"`
	Roles     []Role            `json:"roles,omitempty"`
	Fields    map[string]string `json:"fields"`
	Providers []ProviderRef     `json:"providers,omitempty"`
}

// Edge is a directed relationship between two nodes.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
}

// Graph is the full dossier for one transaction.
type Graph struct {
	Chain  string `json:"chain"`
	TxHash string `json:"txHash"`
	Nodes  []Node `json:"nodes"`
	Edges  []Edge `json:"edges"`
	// Truncated is set when the tx had more events than the dossier cap (128)
	// and the graph shows only the first 128 by log_index.
	Truncated bool `json:"truncated,omitempty"`
}

// Stats is the StatsProvider payload for one address.
type Stats struct {
	Address                string `json:"address"`
	Chain                  string `json:"chain"`
	PaymentCount           int64  `json:"paymentCount"`
	VolumeUSDC             string `json:"volumeUsdc"`
	DistinctCounterparties int64  `json:"distinctCounterparties"`
	FirstSeen              string `json:"firstSeen"`
	LastSeen               string `json:"lastSeen"`
	FacilitatorKnown       bool   `json:"facilitatorKnown"`
	Roles                  []Role `json:"roles"`
}

// LensTotals holds aggregate counts for a single membership lens.
type LensTotals struct {
	TxnCount   int64  `json:"txnCount"`
	VolumeUSDC string `json:"volumeUsdc"`
}

// Meta is the MetaProvider payload: dataset stamp + lens totals.
type Meta struct {
	DataMaxDay         string                `json:"dataMaxDay"`
	BuiltAt            string                `json:"builtAt"`
	MethodologyVersion int                   `json:"methodologyVersion"`
	Totals             map[string]LensTotals `json:"totals"` // keys: "known", "all"
}

// LensSummary is one lens's aggregate for one role.
type LensSummary struct {
	TxnCount               int64  `json:"txnCount"`
	VolumeUSDC             string `json:"volumeUsdc"`
	FirstDay               string `json:"firstDay,omitempty"`
	LastDay                string `json:"lastDay,omitempty"`
	ActiveDays             int64  `json:"activeDays"`
	DistinctCounterparties int64  `json:"distinctCounterparties"`
}

// IdentitySignal is one row of provenance from entity_signal / the view.
type IdentitySignal struct {
	Source    string `json:"source"`
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	URL       string `json:"url,omitempty"`
	FetchedAt string `json:"fetchedAt,omitempty"`
}

// Entity is the /entity/{addr} header payload. Summaries carry BOTH lenses so
// the UI lens toggle never refetches (spec §5).
type Entity struct {
	Chain       string                            `json:"chain"`
	Address     string                            `json:"address"`
	Label       string                            `json:"label,omitempty"`
	LabelSource string                            `json:"labelSource,omitempty"`
	Roles       []string                          `json:"roles"`
	Signals     []IdentitySignal                  `json:"signals,omitempty"`
	Summaries   map[string]map[string]LensSummary `json:"summaries"` // role -> lens -> summary
}

// NeighborRow is one counterparty/facilitator node candidate.
type NeighborRow struct {
	Address    string `json:"address"`
	Label      string `json:"label,omitempty"`
	TxnCount   int64  `json:"txnCount"`
	VolumeUSDC string `json:"volumeUsdc"`
	Share      string `json:"share"` // fraction of the list total, "0.415000"
	FirstSeen  string `json:"firstSeen"`
	LastSeen   string `json:"lastSeen"`
}

// NeighborList carries one direction's top rows plus the honest total count.
type NeighborList struct {
	Total int64         `json:"total"` // distinct counterparties in this direction under the lens
	Rows  []NeighborRow `json:"rows"`
}

// Neighbors is the canvas feed for one address (spec §5): counterparties per
// direction plus the facilitators that settle for it. Empty lists are omitted.
type Neighbors struct {
	Address       string        `json:"address"`
	Lens          string        `json:"lens"`
	Payees        *NeighborList `json:"payees,omitempty"`        // whom it pays
	Payers        *NeighborList `json:"payers,omitempty"`        // who pays it
	Facilitators  *NeighborList `json:"facilitators,omitempty"`  // who settles for it
	SettledPayers *NeighborList `json:"settledPayers,omitempty"` // it settles for these payers
	SettledPayees *NeighborList `json:"settledPayees,omitempty"` // it settles to these payees
}

// DayPoint is one day of one role's activity (sparse; client densifies).
type DayPoint struct {
	Day        string `json:"day"`
	TxnCount   int64  `json:"txnCount"`
	VolumeUSDC string `json:"volumeUsdc"`
}

// Timeline groups sparse day series per role.
type Timeline struct {
	Address string                `json:"address"`
	Lens    string                `json:"lens"`
	Roles   map[string][]DayPoint `json:"roles"`
}

// PricePoint is one amount bucket from entity_price_point_v1.
type PricePoint struct {
	AmountUSDC string `json:"amountUsdc"`
	TxnCount   int64  `json:"txnCount"`
}

// RoleFingerprint is the behavior read for one role under one lens.
type RoleFingerprint struct {
	ActiveDays           int64        `json:"activeDays"`
	SpanDays             int64        `json:"spanDays"`
	MedianTxnsPerDay     int64        `json:"medianTxnsPerDay"`
	TopDayShare          string       `json:"topDayShare"` // "0.041000"
	PricePoints          []PricePoint `json:"pricePoints"`
	TotalDistinctAmounts *int64       `json:"totalDistinctAmounts"` // null when lens=all (not derivable from capped partitions)
	Top1Share            string       `json:"top1Share"`
	Top3Share            string       `json:"top3Share"`
}

// Fingerprint is the ActivityProvider fingerprint payload.
type Fingerprint struct {
	Address string                     `json:"address"`
	Lens    string                     `json:"lens"`
	Roles   map[string]RoleFingerprint `json:"roles"`
}

// CounterpartyQuery parameterises a counterparty list request (Task 7).
type CounterpartyQuery struct {
	Role   string
	Lens   string
	Sort   string
	Limit  int
	Offset int
}

// CounterpartyPage is the ListProvider counterparty page payload.
type CounterpartyPage struct {
	Address string        `json:"address"`
	Role    string        `json:"role"`
	Lens    string        `json:"lens"`
	Total   int64         `json:"total"`
	Rows    []NeighborRow `json:"rows"` // same row shape as neighbors
}

// PaymentQuery parameterises a payment list request (Task 7).
type PaymentQuery struct {
	Role   string
	Lens   string
	Limit  int
	Before string
}

// PaymentRow is one row in a PaymentPage.
type PaymentRow struct {
	TxHash           string `json:"txHash"`
	LogIndex         int64  `json:"logIndex"`
	BlockNumber      int64  `json:"blockNumber"`
	BlockTimestamp   string `json:"blockTimestamp"`
	Payer            string `json:"payer"`
	Payee            string `json:"payee"`
	Facilitator      string `json:"facilitator"`
	AmountUSDC       string `json:"amountUsdc"`
	FacilitatorKnown bool   `json:"facilitatorKnown"`
}

// PaymentPage is the ListProvider payment page payload.
type PaymentPage struct {
	Address string       `json:"address"`
	Role    string       `json:"role"`
	Lens    string       `json:"lens"`
	Rows    []PaymentRow `json:"rows"`
	Next    string       `json:"next,omitempty"` // cursor for the next page
}

// LeaderboardRow is one entity in a ranked leaderboard slice.
type LeaderboardRow struct {
	Rank                   int    `json:"rank"`
	Address                string `json:"address"`
	Label                  string `json:"label,omitempty"`
	TxnCount               int64  `json:"txnCount"`
	VolumeUSDC             string `json:"volumeUsdc"`
	DistinctCounterparties int64  `json:"distinctCounterparties"`
	FirstSeen              string `json:"firstSeen"`
	LastSeen               string `json:"lastSeen"`
}

// Leaderboard is the LeaderboardProvider payload (Task 8).
type Leaderboard struct {
	Role   string           `json:"role"`
	Window string           `json:"window"`
	Lens   string           `json:"lens"`
	Sort   string           `json:"sort"`
	Rows   []LeaderboardRow `json:"rows"`
}
