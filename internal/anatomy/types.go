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

// Stub types filled in by Tasks 4-7 (fields added there; declaring them now
// lets the provider seam land in one piece).

// Entity is the EntityProvider payload for one address (Task 4).
type Entity struct{}

// Neighbors is the NeighborProvider payload for one address (Task 5).
type Neighbors struct{}

// Timeline is the ActivityProvider timeline payload (Task 6).
type Timeline struct{}

// Fingerprint is the ActivityProvider fingerprint payload (Task 6).
type Fingerprint struct{}

// CounterpartyQuery parameterises a counterparty list request (Task 7).
type CounterpartyQuery struct {
	Role   string
	Lens   string
	Sort   string
	Limit  int
	Offset int
}

// CounterpartyPage is the ListProvider counterparty page payload (Task 7).
type CounterpartyPage struct{}

// PaymentQuery parameterises a payment list request (Task 7).
type PaymentQuery struct {
	Role   string
	Lens   string
	Limit  int
	Before string
}

// PaymentPage is the ListProvider payment page payload (Task 7).
type PaymentPage struct{}

// Leaderboard is the LeaderboardProvider payload (Task 8).
type Leaderboard struct{}
