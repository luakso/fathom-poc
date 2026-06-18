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
