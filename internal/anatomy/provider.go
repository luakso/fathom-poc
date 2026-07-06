package anatomy

import "context"

// DossierProvider builds the dossier graph for one transaction.
type DossierProvider interface {
	Dossier(ctx context.Context, chain, txHash string) (Graph, error)
}

// EntityProvider returns entity header data for one address.
type EntityProvider interface {
	Entity(ctx context.Context, chain, address string) (Entity, error)
}

// NeighborProvider returns the graph neighborhood of one address.
type NeighborProvider interface {
	Neighbors(ctx context.Context, chain, address string, lens Lens, limit int) (Neighbors, error)
}

// ActivityProvider returns timeline and fingerprint activity for one address.
type ActivityProvider interface {
	Timeline(ctx context.Context, chain, address string, lens Lens) (Timeline, error)
	Fingerprint(ctx context.Context, chain, address string, lens Lens) (Fingerprint, error)
}

// ListProvider returns paginated counterparty and payment lists for one address.
type ListProvider interface {
	Counterparties(ctx context.Context, chain, address string, q CounterpartyQuery) (CounterpartyPage, error)
	Payments(ctx context.Context, chain, address string, q PaymentQuery) (PaymentPage, error)
}

// LeaderboardProvider returns ranked entity lists.
type LeaderboardProvider interface {
	Leaderboard(ctx context.Context, chain string, role Role, window Window, lens Lens, sort Sort) (Leaderboard, error)
}

// MetaProvider returns dataset metadata (stamp + totals).
type MetaProvider interface {
	Meta(ctx context.Context) (Meta, error)
}

// Providers bundles everything NewServer needs; nil fields are allowed until
// their routes are wired (handlers for nil providers return 404).
type Providers struct {
	Dossier     DossierProvider
	Entity      EntityProvider
	Neighbors   NeighborProvider
	Activity    ActivityProvider
	Lists       ListProvider
	Leaderboard LeaderboardProvider
	Meta        MetaProvider
}
