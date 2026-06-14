//go:build integration

package base_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMigration_CaptureColumnsAndView asserts the 00011 migration added the
// curated capture columns to payments and created the payment_x402_v1 view.
func TestMigration_CaptureColumnsAndView(t *testing.T) {
	ctx, store := setup(t)

	wantCols := []string{
		"settlement_kind", "self_settled", "valid_after", "valid_before",
		"input_calldata", "block_hash", "transaction_index",
		"token_decimals", "token_symbol", "payer_account_type",
	}
	for _, col := range wantCols {
		var exists bool
		require.NoError(t, store.Pool().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'payments' AND column_name = $1
			)`, col).Scan(&exists))
		require.True(t, exists, "payments.%s must exist after migration 00011", col)
	}

	// The view is selectable and exposes facilitator_known.
	_, err := store.Pool().Exec(ctx, `SELECT facilitator_known FROM payment_x402_v1 WHERE false`)
	require.NoError(t, err, "payment_x402_v1 must expose facilitator_known")
}
