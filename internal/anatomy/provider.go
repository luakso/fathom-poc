package anatomy

import "context"

// DossierProvider builds the dossier graph for one transaction.
type DossierProvider interface {
	Dossier(ctx context.Context, chain, txHash string) (Graph, error)
}

// StatsProvider returns aggregate statistics for one address.
type StatsProvider interface {
	Stats(ctx context.Context, chain, address string) (Stats, error)
}

// EntityProvider returns entity header data for one address.
type EntityProvider interface {
	Entity(ctx context.Context, chain, address string) (Entity, error)
}

// NeighborProvider returns the graph neighborhood of one address.
type NeighborProvider interface {
	Neighbors(ctx context.Context, chain, address, lens string, limit int) (Neighbors, error)
}

// ActivityProvider returns timeline and fingerprint activity for one address.
type ActivityProvider interface {
	Timeline(ctx context.Context, chain, address, lens string) (Timeline, error)
	Fingerprint(ctx context.Context, chain, address, lens string) (Fingerprint, error)
}

// ListProvider returns paginated counterparty and payment lists for one address.
type ListProvider interface {
	Counterparties(ctx context.Context, chain, address string, q CounterpartyQuery) (CounterpartyPage, error)
	Payments(ctx context.Context, chain, address string, q PaymentQuery) (PaymentPage, error)
}

// LeaderboardProvider returns ranked entity lists.
type LeaderboardProvider interface {
	Leaderboard(ctx context.Context, chain, role, window, lens, sort string) (Leaderboard, error)
}

// MetaProvider returns dataset metadata (stamp + totals).
type MetaProvider interface {
	Meta(ctx context.Context) (Meta, error)
}

// Providers bundles everything NewServer needs; nil fields are allowed until
// their routes are wired (handlers for nil providers return 404).
type Providers struct {
	Dossier     DossierProvider
	Stats       StatsProvider // legacy v1 endpoint, removed in Plan C
	Entity      EntityProvider
	Neighbors   NeighborProvider
	Activity    ActivityProvider
	Lists       ListProvider
	Leaderboard LeaderboardProvider
	Meta        MetaProvider
}
