//go:build integration

package anatomy_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// resolveIdentity reads the single resolved identity row for an address.
func resolveIdentity(t *testing.T, ctx context.Context, db *sql.DB, addr string) (source, label string) {
	t.Helper()
	err := db.QueryRowContext(ctx,
		`SELECT source, label FROM entity_identity_v1 WHERE chain = 'base' AND address = $1`,
		addr).Scan(&source, &label)
	require.NoError(t, err)
	return source, label
}

func TestIdentityView_PrecedenceAndAllowlistFallback(t *testing.T) {
	ctx, db, _ := setupAnatomy(t)

	// Allowlist-only address resolves via allowlist.
	_, err := db.ExecContext(ctx, `
		INSERT INTO facilitator_allowlist (chain, address, source, label, since_version)
		VALUES ('base', '0xfac1', 'manual', 'TestFacilitator', 1)`)
	require.NoError(t, err)
	src, label := resolveIdentity(t, ctx, db, "0xfac1")
	require.Equal(t, "allowlist", src)
	require.Equal(t, "TestFacilitator", label)

	// A manual signal beats the allowlist label for the same address.
	_, err = db.ExecContext(ctx, `
		INSERT INTO entity_signal (chain, address, source, kind, value)
		VALUES ('base', '0xfac1', 'manual', 'label', 'My Override')`)
	require.NoError(t, err)
	src, label = resolveIdentity(t, ctx, db, "0xfac1")
	require.Equal(t, "manual", src)
	require.Equal(t, "My Override", label)

	// A catalog signal beats a basename signal.
	for _, row := range [][3]string{
		{"basename", "name", "cool.base.eth"},
		{"catalog", "endpoint", "api.example.com"},
	} {
		_, err = db.ExecContext(ctx, `
			INSERT INTO entity_signal (chain, address, source, kind, value)
			VALUES ('base', '0xpayee1', $1, $2, $3)`, row[0], row[1], row[2])
		require.NoError(t, err)
	}
	src, label = resolveIdentity(t, ctx, db, "0xpayee1")
	require.Equal(t, "catalog", src)
	require.Equal(t, "api.example.com", label)

	// Unknown address: no row.
	var n int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT count(*) FROM entity_identity_v1 WHERE address = '0xnobody'`).Scan(&n))
	require.Equal(t, 0, n)
}
