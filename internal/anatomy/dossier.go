package anatomy

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgDossier builds dossier graphs from the payment_x402_v1 view.
type PgDossier struct{ pool *pgxpool.Pool }

var _ DossierProvider = (*PgDossier)(nil)

// NewPgDossier returns a PgDossier reading from pool.
func NewPgDossier(pool *pgxpool.Pool) *PgDossier { return &PgDossier{pool: pool} }

type dossierRow struct {
	logIndex         int
	blockNumber      int64
	blockTimestamp   string
	txType           int16
	txNonce          int64
	calledContract   string
	methodSelector   []byte
	gasUsed          int64
	gasCostWei       string
	authNonce        []byte
	amountUSDC       string
	asset            string
	tokenSymbol      *string
	settlementKind   string
	selfSettled      bool
	facilitatorKnown bool
	facilitator      string
	payer            string
	payee            string
}

// Dossier implements DossierProvider.
func (p *PgDossier) Dossier(ctx context.Context, chain, txHash string) (Graph, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT log_index, block_number, block_timestamp::text, tx_type, tx_nonce,
		       called_contract, method_selector, gas_used, gas_cost_wei::text,
		       auth_nonce, amount_usdc::text, asset, token_symbol, settlement_kind,
		       self_settled, facilitator_known, facilitator, payer, payee
		FROM payment_x402_v1
		WHERE chain = $1 AND tx_hash = $2
		ORDER BY log_index`, chain, txHash)
	if err != nil {
		return Graph{}, fmt.Errorf("query dossier: %w", err)
	}
	defer rows.Close()

	var rs []dossierRow
	for rows.Next() {
		var r dossierRow
		if err := rows.Scan(&r.logIndex, &r.blockNumber, &r.blockTimestamp, &r.txType,
			&r.txNonce, &r.calledContract, &r.methodSelector, &r.gasUsed, &r.gasCostWei,
			&r.authNonce, &r.amountUSDC, &r.asset, &r.tokenSymbol, &r.settlementKind,
			&r.selfSettled, &r.facilitatorKnown, &r.facilitator, &r.payer, &r.payee); err != nil {
			return Graph{}, fmt.Errorf("scan dossier row: %w", err)
		}
		rs = append(rs, r)
	}
	if err := rows.Err(); err != nil {
		return Graph{}, fmt.Errorf("iterate dossier rows: %w", err)
	}
	if len(rs) == 0 {
		return Graph{}, ErrNotFound
	}
	return buildGraph(chain, txHash, rs), nil
}

func buildGraph(chain, txHash string, rs []dossierRow) Graph {
	g := Graph{Chain: chain, TxHash: txHash}
	first := rs[0]
	txID := "tx:" + txHash
	g.Nodes = append(g.Nodes, Node{
		ID: txID, Kind: NodeTransaction, Label: txHash,
		Fields: map[string]string{
			"block":          fmt.Sprintf("%d", first.blockNumber),
			"timestamp":      first.blockTimestamp,
			"txType":         fmt.Sprintf("%d", first.txType),
			"txNonce":        fmt.Sprintf("%d", first.txNonce),
			"calledContract": first.calledContract,
			"methodSelector": "0x" + hex.EncodeToString(first.methodSelector),
			"gasUsed":        fmt.Sprintf("%d", first.gasUsed),
			"gasCostWei":     first.gasCostWei,
		},
	})

	addrIdx := map[string]int{} // address -> index in g.Nodes
	edgeSeen := map[string]bool{}
	addAddr := func(addr string, role Role) string {
		id := "addr:" + addr
		if i, ok := addrIdx[addr]; ok {
			if !slices.Contains(g.Nodes[i].Roles, role) {
				g.Nodes[i].Roles = append(g.Nodes[i].Roles, role)
			}
			return id
		}
		addrIdx[addr] = len(g.Nodes)
		g.Nodes = append(g.Nodes, Node{
			ID: id, Kind: NodeAddress, Label: addr, Roles: []Role{role},
			Fields: map[string]string{"address": addr},
			Providers: []ProviderRef{
				{Kind: "stats", Available: true},
				{Kind: "identity", Available: false},
				{Kind: "onchain", Available: false},
				{Kind: "internet", Available: false},
			},
		})
		return id
	}
	addEdge := func(e Edge) {
		if edgeSeen[e.ID] {
			return
		}
		edgeSeen[e.ID] = true
		g.Edges = append(g.Edges, e)
	}

	for _, r := range rs {
		evtID := fmt.Sprintf("evt:%s:%d", txHash, r.logIndex)
		sym := ""
		if r.tokenSymbol != nil {
			sym = *r.tokenSymbol
		}
		g.Nodes = append(g.Nodes, Node{
			ID: evtID, Kind: NodeEvent, Label: fmt.Sprintf("log %d", r.logIndex),
			Fields: map[string]string{
				"amountUsdc":       r.amountUSDC,
				"asset":            r.asset,
				"tokenSymbol":      sym,
				"settlementKind":   r.settlementKind,
				"selfSettled":      fmt.Sprintf("%t", r.selfSettled),
				"facilitatorKnown": fmt.Sprintf("%t", r.facilitatorKnown),
				"authNonce":        "0x" + hex.EncodeToString(r.authNonce),
			},
		})
		payerID := addAddr(r.payer, RolePayer)
		payeeID := addAddr(r.payee, RolePayee)
		facID := addAddr(r.facilitator, RoleFacilitator)

		addEdge(Edge{ID: "e:emits:" + evtID, Source: txID, Target: evtID, Kind: "emits"})
		addEdge(Edge{
			ID:     fmt.Sprintf("e:pays:%s:%s:%d", r.payer, r.payee, r.logIndex),
			Source: payerID, Target: payeeID, Label: r.amountUSDC, Kind: "pays",
		})
		addEdge(Edge{ID: "e:settles:" + evtID, Source: facID, Target: evtID, Kind: "settles"})
	}
	return g
}
