//go:build integration

package base_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/db"
	"github.com/lukostrobl/fathom/internal/x402"
)

// setupClassified starts an ephemeral Postgres, applies migrations AND the
// views/ directory (mirroring database/init/init-db.sh), and returns a raw
// *sql.DB (for dimension rows + view queries) alongside a *base.Store (to insert
// payment fixtures through the real write path).
func setupClassified(t *testing.T) (context.Context, *sql.DB, *base.Store) {
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

	// Mirror init-db: apply views/*.sql after migrations, in filename order.
	views, err := filepath.Glob("../../database/views/*.sql")
	require.NoError(t, err)
	sort.Strings(views)
	for _, v := range views {
		b, err := os.ReadFile(v)
		require.NoError(t, err)
		_, err = sqlDB.ExecContext(ctx, string(b))
		require.NoError(t, err, "applying view %s", filepath.Base(v))
	}

	pool, err := db.Open(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return ctx, sqlDB, base.NewStore(pool)
}

// All test addresses are lowercase to match the stored (lowercased) principals.
const (
	usdcToken = "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"
	facAllow  = "0xa11ce00000000000000000000000000000000001" // allowlisted at v1
	facOther  = "0x0ther00000000000000000000000000000000002" // never allowlisted
	facV2Only = "0xb0b0000000000000000000000000000000000003" // allowlisted at v2 only
	denyV1    = "0xdead00000000000000000000000000000000d001" // denylisted at v1
)

func paymentAt(logIndex uint32, txHash, facilitator, calledContract string) x402.Payment {
	p := samplePayment(logIndex)
	p.TxHash = txHash
	p.Facilitator = facilitator
	p.CalledContract = calledContract
	return p
}

func TestClassification_AttributionLabels(t *testing.T) {
	ctx, sqlDB, store := setupClassified(t)

	// Dimension rows (test-local; independent of the seeded v1 denylist).
	_, err := sqlDB.ExecContext(ctx,
		`INSERT INTO facilitator_allowlist (chain, address, source, since_version) VALUES ('base',$1,'test',1)`, facAllow)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO facilitator_allowlist (chain, address, source, since_version) VALUES ('base',$1,'test',2)`, facV2Only)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx,
		`INSERT INTO contamination_denylist (chain, called_contract, label, reason, confidence, since_version)
		 VALUES ('base',$1,'test-deny','test','confirmed',1)`, denyV1)
	require.NoError(t, err)

	// Payment fixtures, one per expected label.
	batch := []x402.Payment{
		paymentAt(1, "0xagentic", facAllow, usdcToken),   // allowlisted payer, clean venue
		paymentAt(2, "0xcontam", facAllow, denyV1),       // denylist wins over allowlisted payer
		paymentAt(3, "0xcontested", facOther, usdcToken), // unknown payer, clean venue
		paymentAt(4, "0xv2only", facV2Only, usdcToken),   // payer only allowlisted at v2
	}
	require.NoError(t, store.InsertBatch(ctx, batch, nil, 100))

	got := map[string]string{}
	rows, err := sqlDB.QueryContext(ctx, `SELECT tx_hash, attribution FROM payment_classified_v1`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var tx, attr string
		require.NoError(t, rows.Scan(&tx, &attr))
		got[tx] = attr
	}
	require.NoError(t, rows.Err())

	require.Equal(t, "agentic", got["0xagentic"])
	require.Equal(t, "contamination", got["0xcontam"], "denylisted tx.to must win over an allowlisted tx.from")
	require.Equal(t, "contested", got["0xcontested"])
	require.Equal(t, "contested", got["0xv2only"], "a since_version=2 facilitator is invisible to the v1 view")
}

func TestClassification_ViewNeverDropsRows(t *testing.T) {
	ctx, sqlDB, store := setupClassified(t)

	batch := []x402.Payment{
		paymentAt(1, "0xone", facOther, usdcToken),
		paymentAt(2, "0xtwo", facOther, denyV1),
		paymentAt(3, "0xthree", facOther, usdcToken),
	}
	require.NoError(t, store.InsertBatch(ctx, batch, nil, 100))

	var nView, nPay int
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM payment_classified_v1`).Scan(&nView))
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM payments`).Scan(&nPay))
	require.Equal(t, nPay, nView, "the classification view must be a label, never a filter")
}
