//go:build integration

package anatomy_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/anatomy"
)

func setupAnatomy(t *testing.T) (context.Context, *sql.DB, *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	pg, err := postgres.Run(
		ctx, "postgres:16-alpine",
		postgres.WithDatabase("fathom_test"),
		postgres.WithUsername("fathom"),
		postgres.WithPassword("fathom"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })
	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	sqlDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { sqlDB.Close() })
	require.NoError(t, goose.SetDialect("postgres"))
	require.NoError(t, goose.Up(sqlDB, "../../database/migrations"))
	views, err := filepath.Glob("../../database/views/*.sql")
	require.NoError(t, err)
	sort.Strings(views)
	for _, v := range views {
		b, err := os.ReadFile(v)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, string(b))
		require.NoError(t, err, "applying view %s", filepath.Base(v))
	}
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return ctx, sqlDB, pool
}

// seedPayment inserts one payment row with the columns the dossier reads.
// amount_usdc is a GENERATED column (amount_raw * 0.000001), so it is omitted
// from the INSERT; amount_raw is set to amt * 1e6 so the generated value matches.
func seedPayment(t *testing.T, ctx context.Context, db *sql.DB, txHash string, logIdx int, payer, facilitator, payee, amt string) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO payments (
			chain, tx_hash, log_index, block_number, block_timestamp, source, protocol,
			facilitator, payer, payee, asset, token_address, amount_raw,
			asset_usd_at_time, auth_nonce, method_selector, called_contract, tx_type,
			tx_nonce, gas_used, effective_gas_price, gas_cost_wei
		) VALUES (
			'base', $1, $2, 100, '2026-06-01T10:00:00Z', 'test', 'x402',
			$3, $4, $5, 'USDC', '0xtoken', ($6::numeric * 1000000)::numeric(78,0),
			1.0, '\x01', '\xdeadbeef', '0xcontract', 2,
			7, 50000, 1000000, 50000000
		)`, txHash, logIdx, facilitator, payer, payee, amt)
	require.NoError(t, err)
}

func TestDossier_SinglePayment(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedPayment(t, ctx, db, "0xtx1", 0, "0xpayer", "0xfac", "0xpayee", "2.50")

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtx1")
	require.NoError(t, err)
	require.Equal(t, "0xtx1", g.TxHash)

	kinds := map[anatomy.NodeKind]int{}
	for _, n := range g.Nodes {
		kinds[n.Kind]++
	}
	require.Equal(t, 1, kinds[anatomy.NodeTransaction])
	require.Equal(t, 1, kinds[anatomy.NodeEvent])
	require.Equal(t, 3, kinds[anatomy.NodeAddress], "payer, payee, facilitator")
	require.Len(t, g.Edges, 3)

	var payerNode *anatomy.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "addr:0xpayer" {
			payerNode = &g.Nodes[i]
		}
	}
	require.NotNil(t, payerNode)
	require.Equal(t, []anatomy.ProviderRef{
		{Kind: "stats", Available: true},
		{Kind: "identity", Available: false},
		{Kind: "onchain", Available: false},
		{Kind: "internet", Available: false},
	}, payerNode.Providers)
}

func TestDossier_NotFound(t *testing.T) {
	ctx, _, pool := setupAnatomy(t)
	_, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xmissing")
	require.ErrorIs(t, err, anatomy.ErrNotFound)
}

func TestDossier_DedupAddressRoles(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	// payer and facilitator are the same address → one node, two roles.
	seedPayment(t, ctx, db, "0xtx2", 0, "0xsame", "0xsame", "0xpayee", "1.00")
	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtx2")
	require.NoError(t, err)
	var same *anatomy.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "addr:0xsame" {
			same = &g.Nodes[i]
		}
	}
	require.NotNil(t, same)
	require.ElementsMatch(t, []anatomy.Role{anatomy.RolePayer, anatomy.RoleFacilitator}, same.Roles)
}

// seedPaymentFull inserts one row with all TX-level columns the enriched dossier
// reads. l1_fee/gas_limit/etc. are set so the panel fields and total-fee math are
// exercised. amount_usdc is GENERATED from amount_raw.
func seedPaymentFull(t *testing.T, ctx context.Context, db *sql.DB, txHash string, logIdx int, payer, facilitator, payee, amt, selectorHex string) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		INSERT INTO payments (
			chain, tx_hash, log_index, block_number, block_timestamp, source, protocol,
			facilitator, payer, payee, asset, token_address, amount_raw,
			asset_usd_at_time, auth_nonce, method_selector, called_contract, tx_type,
			tx_nonce, gas_used, effective_gas_price, gas_cost_wei,
			gas_limit, base_fee_per_gas, max_fee_per_gas, max_priority_fee_per_gas,
			l1_fee, l1_gas_used, l1_gas_price, tx_value, transaction_index, block_hash,
			input_calldata, token_symbol, token_decimals, valid_after, valid_before
		) VALUES (
			'base', $1, $2, 100, '2026-06-01T10:00:00Z', 'test', 'x402',
			$3, $4, $5, 'USDC', '0x833589fcd6edb6e08f4c7c32d4f71b54bda02913',
			($6::numeric * 1000000)::numeric(78,0),
			1.0, '\x01', decode($7,'hex'), '0x833589fcd6edb6e08f4c7c32d4f71b54bda02913', 2,
			7, 85720, 1000000, 8572000000000,
			95307, 5000000, 30000000, 5000000,
			2617501899, 3307, 123539468, 0, 51, '0xblockhash',
			'\xdeadbeef', 'USDC', 6, NULL, '2026-06-01T11:00:00Z'
		)`, txHash, logIdx, facilitator, payer, payee, amt, selectorHex)
	require.NoError(t, err)
}

func fieldsOf(g anatomy.Graph) map[string]string {
	for _, n := range g.Nodes {
		if n.Kind == anatomy.NodeTransaction {
			return n.Fields
		}
	}
	return nil
}

func TestDossier_TxFieldsEnriched(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedPaymentFull(t, ctx, db, "0xtxE", 0, "0xpayer", "0xfac", "0xpayee", "0.002", "e3ee160e")

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtxE")
	require.NoError(t, err)
	f := fieldsOf(g)
	require.NotNil(t, f)

	require.Equal(t, "0.002000", f["paid"])             // SUM(amount_usdc)
	require.Equal(t, "8574617501899", f["totalFeeWei"]) // gas_cost_wei + l1_fee
	require.Equal(t, "transferWithAuthorization", f["method"])
	require.Equal(t, "v,r,s", f["methodKind"])
	require.Equal(t, "0xe3ee160e", f["methodId"])
	require.Equal(t, "USDC · Circle", f["contractLabel"])
	require.Equal(t, "success", f["status"])
	require.Equal(t, "1", f["eventCount"])
	require.Equal(t, "95307", f["gasLimit"])
	require.Equal(t, "https://basescan.org/tx/0xtxE", f["explorerUrl"])
	require.Equal(t, "true", f["decodable"]) // single 3009 call
	require.Equal(t, "0xpayer", f["dpFrom"])
	require.Equal(t, "0xpayee", f["dpTo"])
}

func TestDossier_MultiEventNotDecodable(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedPaymentFull(t, ctx, db, "0xtxM", 0, "0xp1", "0xfac", "0xq1", "1.00", "82ad56cb")
	seedPaymentFull(t, ctx, db, "0xtxM", 1, "0xp2", "0xfac", "0xq2", "2.00", "82ad56cb")

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtxM")
	require.NoError(t, err)
	f := fieldsOf(g)
	require.Equal(t, "3.000000", f["paid"]) // 1 + 2
	require.Equal(t, "2", f["eventCount"])
	require.Equal(t, "aggregate3", f["method"])
	require.Equal(t, "false", f["decodable"])
}

func TestDossier_FacilitatorIdentity(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	// 0x67b9ce70… is the "Coinbase" facilitator in the migration-seeded allowlist.
	const coinbase = "0x67b9ce703d9ce658d7c4ac3c289cea112fe662af"
	seedPaymentFull(t, ctx, db, "0xtxID", 0, "0xpayer", coinbase, "0xpayee", "0.002", "e3ee160e")

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtxID")
	require.NoError(t, err)

	var fac *anatomy.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "addr:"+coinbase {
			fac = &g.Nodes[i]
		}
	}
	require.NotNil(t, fac, "facilitator node present")
	require.Equal(t, "Coinbase", fac.Fields["entityLabel"])
	require.Equal(t, "true", fac.Fields["facilitatorKnown"])
	require.Equal(t, "false", fac.Fields["selfSettled"])
}

func TestDossier_UnknownFacilitatorNoLabel(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	seedPaymentFull(t, ctx, db, "0xtxU", 0, "0xpayer", "0xnotallowlisted", "0xpayee", "1.00", "e3ee160e")

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtxU")
	require.NoError(t, err)
	var fac *anatomy.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "addr:0xnotallowlisted" {
			fac = &g.Nodes[i]
		}
	}
	require.NotNil(t, fac)
	require.Equal(t, "", fac.Fields["entityLabel"])
	require.Equal(t, "false", fac.Fields["facilitatorKnown"])
	require.Equal(t, "false", fac.Fields["selfSettled"])
}

func TestDossier_SelfSettledFlag(t *testing.T) {
	ctx, db, pool := setupAnatomy(t)
	// Direct insert so self_settled is true (seedPaymentFull defaults it false).
	_, err := db.ExecContext(ctx, `
		INSERT INTO payments (
			chain, tx_hash, log_index, block_number, block_timestamp, source, protocol,
			facilitator, payer, payee, asset, token_address, amount_raw,
			asset_usd_at_time, auth_nonce, method_selector, called_contract, tx_type,
			tx_nonce, gas_used, effective_gas_price, gas_cost_wei, self_settled
		) VALUES (
			'base','0xtxS',0,100,'2026-06-01T10:00:00Z','test','x402',
			'0xselffac','0xselffac','0xpayee','USDC','0xtoken',1000000,
			1.0,'\x01','\xe3ee160e','0xcontract',2,
			7,50000,1000000,50000000, true
		)`)
	require.NoError(t, err)

	g, err := anatomy.NewPgDossier(pool).Dossier(ctx, "base", "0xtxS")
	require.NoError(t, err)
	var fac *anatomy.Node
	for i := range g.Nodes {
		if g.Nodes[i].ID == "addr:0xselffac" {
			fac = &g.Nodes[i]
		}
	}
	require.NotNil(t, fac)
	require.Equal(t, "true", fac.Fields["selfSettled"])
}
