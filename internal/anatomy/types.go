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
