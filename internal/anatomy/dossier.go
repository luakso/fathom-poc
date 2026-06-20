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
	blockHash        *string
	txType           int16
	txNonce          int64
	txIndex          *string
	calledContract   string
	methodSelector   []byte
	gasUsed          int64
	gasLimit         *string
	effGasPrice      string
	gasCostWei       string
	baseFee          *string
	maxFee           *string
	maxPriorityFee   *string
	l1Fee            *string
	l1GasUsed        *string
	l1GasPrice       *string
	txValue          *string
	inputCalldata    []byte
	authNonce        []byte
	amountUSDC       string
	validAfter       *string
	validBefore      *string
	asset            string
	tokenSymbol      *string
	tokenDecimals    *string
	settlementKind   string
	selfSettled      bool
	facilitatorKnown bool
	facilitator      string
	payer            string
	payee            string
	// window aggregates (identical on every row of the tx)
	paidTotal   string
	totalFeeWei string
}

// Dossier implements DossierProvider.
func (p *PgDossier) Dossier(ctx context.Context, chain, txHash string) (Graph, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT log_index, block_number, block_timestamp::text, block_hash,
		       tx_type, tx_nonce, transaction_index::text,
		       called_contract, method_selector, gas_used, gas_limit::text,
		       effective_gas_price::text, gas_cost_wei::text,
		       base_fee_per_gas::text, max_fee_per_gas::text, max_priority_fee_per_gas::text,
		       l1_fee::text, l1_gas_used::text, l1_gas_price::text, tx_value::text,
		       input_calldata, auth_nonce, amount_usdc::text,
		       valid_after::text, valid_before::text,
		       asset, token_symbol, token_decimals::text, settlement_kind,
		       self_settled, facilitator_known, facilitator, payer, payee,
		       (SUM(amount_usdc) OVER ())::text AS paid_total,
		       (gas_cost_wei + COALESCE(l1_fee,0))::text AS total_fee_wei
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
		if err := rows.Scan(&r.logIndex, &r.blockNumber, &r.blockTimestamp, &r.blockHash,
			&r.txType, &r.txNonce, &r.txIndex,
			&r.calledContract, &r.methodSelector, &r.gasUsed, &r.gasLimit,
			&r.effGasPrice, &r.gasCostWei,
			&r.baseFee, &r.maxFee, &r.maxPriorityFee,
			&r.l1Fee, &r.l1GasUsed, &r.l1GasPrice, &r.txValue,
			&r.inputCalldata, &r.authNonce, &r.amountUSDC,
			&r.validAfter, &r.validBefore,
			&r.asset, &r.tokenSymbol, &r.tokenDecimals, &r.settlementKind,
			&r.selfSettled, &r.facilitatorKnown, &r.facilitator, &r.payer, &r.payee,
			&r.paidTotal, &r.totalFeeWei); err != nil {
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

// deref returns the pointed-to string, or "" if nil.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func buildGraph(chain, txHash string, rs []dossierRow) Graph {
	g := Graph{Chain: chain, TxHash: txHash}
	first := rs[0]
	txID := "tx:" + txHash

	name, kind, _ := MethodName(first.methodSelector)
	is3009 := name == "transferWithAuthorization"
	decodable := is3009 && len(rs) == 1

	fields := map[string]string{
		"paid":              first.paidTotal,
		"totalFeeWei":       first.totalFeeWei,
		"method":            name,
		"methodKind":        kind,
		"methodId":          "0x" + hex.EncodeToString(first.methodSelector),
		"block":             fmt.Sprintf("%d", first.blockNumber),
		"timestamp":         first.blockTimestamp,
		"blockHash":         deref(first.blockHash),
		"from":              first.facilitator,
		"calledContract":    first.calledContract,
		"contractLabel":     ContractLabel(chain, first.calledContract),
		"txValue":           deref(first.txValue),
		"gasUsed":           fmt.Sprintf("%d", first.gasUsed),
		"gasLimit":          deref(first.gasLimit),
		"effectiveGasPrice": first.effGasPrice,
		"baseFee":           deref(first.baseFee),
		"maxFee":            deref(first.maxFee),
		"maxPriorityFee":    deref(first.maxPriorityFee),
		"gasCostWei":        first.gasCostWei,
		"l1Fee":             deref(first.l1Fee),
		"l1GasUsed":         deref(first.l1GasUsed),
		"l1GasPrice":        deref(first.l1GasPrice),
		"txType":            fmt.Sprintf("%d", first.txType),
		"txNonce":           fmt.Sprintf("%d", first.txNonce),
		"transactionIndex":  deref(first.txIndex),
		"tokenSymbol":       deref(first.tokenSymbol),
		"tokenDecimals":     deref(first.tokenDecimals),
		"eventCount":        fmt.Sprintf("%d", len(rs)),
		"inputCalldata":     "0x" + hex.EncodeToString(first.inputCalldata),
		"status":            StatusSuccess,
		"explorerUrl":       ExplorerTxURL(chain, txHash),
		"decodable":         fmt.Sprintf("%t", decodable),
	}
	if decodable {
		fields["dpFrom"] = first.payer
		fields["dpTo"] = first.payee
		fields["dpValue"] = first.amountUSDC
		fields["dpValidAfter"] = deref(first.validAfter)
		fields["dpValidBefore"] = deref(first.validBefore)
		fields["dpNonce"] = "0x" + hex.EncodeToString(first.authNonce)
	}

	g.Nodes = append(g.Nodes, Node{
		ID: txID, Kind: NodeTransaction, Label: txHash, Fields: fields,
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
