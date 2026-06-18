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
