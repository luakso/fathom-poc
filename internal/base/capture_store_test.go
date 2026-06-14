//go:build integration

package base_test

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/x402"
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

// TestStore_CaptureFields_RoundTrip proves the new v2 capture columns persist
// through the COPY → INSERT path, including a nullable window (valid_before set,
// valid_after nil).
func TestStore_CaptureFields_RoundTrip(t *testing.T) {
	ctx, store := setup(t)

	vb := time.Unix(1_700_003_600, 0).UTC()

	p := x402.Payment{
		Chain: x402.ChainBase, TxHash: "0xcap", LogIndex: 1,
		BlockNumber: 100, BlockTimestamp: time.Unix(1_700_000_000, 0).UTC(),
		Source: "base-collector", Protocol: "x402",
		Facilitator: "0xfac", Payer: "0xpay", Payee: "0xrec",
		Asset: "USDC", TokenAddress: strings.ToLower(x402.USDCProxyBase.Hex()),
		AmountRaw: big.NewInt(1_000_000), AssetUSDAtTime: decimalOne(),
		AuthNonce: []byte{0x01}, MethodSelector: []byte{0xe3, 0xee, 0x16, 0x0e},
		CalledContract: strings.ToLower(x402.USDCProxyBase.Hex()),
		TxType:         2, TxNonce: 7, GasUsed: 50_000,
		EffectiveGasPrice: big.NewInt(1_000_000_000), GasCostWei: big.NewInt(50_000_000_000_000),
		// v2 capture fields under test:
		SettlementKind: "receive", SelfSettled: true,
		ValidAfter: nil, ValidBefore: &vb,
		InputCalldata: []byte{0xe3, 0xee, 0x16, 0x0e, 0xde, 0xad},
		BlockHash:     "0xb100", TransactionIndex: 5,
		TokenDecimals: 6, TokenSymbol: "USDC",
	}

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{p}, 100))

	var (
		settlementKind, blockHash, tokenSymbol string
		selfSettled                            bool
		validBefore                            time.Time
		validAfter                             *time.Time
		inputCalldata                          []byte
		txIndex                                int32
		tokenDecimals                          int16
		payerAccountType                       *string
	)
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT settlement_kind, self_settled, valid_after, valid_before,
		       input_calldata, block_hash, transaction_index, token_decimals, token_symbol,
		       payer_account_type
		FROM payments WHERE chain = $1 AND tx_hash = $2 AND log_index = $3`,
		string(x402.ChainBase), "0xcap", int32(1)).Scan(
		&settlementKind, &selfSettled, &validAfter, &validBefore,
		&inputCalldata, &blockHash, &txIndex, &tokenDecimals, &tokenSymbol, &payerAccountType))

	require.Equal(t, "receive", settlementKind)
	require.True(t, selfSettled)
	require.Nil(t, validAfter, "nullable valid_after stays NULL")
	require.True(t, vb.Equal(validBefore), "valid_before round-trips")
	require.Equal(t, []byte{0xe3, 0xee, 0x16, 0x0e, 0xde, 0xad}, inputCalldata)
	require.Equal(t, "0xb100", blockHash)
	require.Equal(t, int32(5), txIndex)
	require.Equal(t, int16(6), tokenDecimals)
	require.Equal(t, "USDC", tokenSymbol)
	require.Nil(t, payerAccountType, "payer_account_type stays NULL until a later plan populates it")
}

// decimalOne is a tiny local helper for the value 1 (avoids importing shopspring
// at every call site).
func decimalOne() decimal.Decimal { return decimal.NewFromInt(1) }

// TestStore_X402View_FacilitatorKnown proves the v2 read view labels a payment
// known vs unknown by the facilitator allowlist. Seeds one allowlist address
// directly to avoid coupling to the migration seed contents.
func TestStore_X402View_FacilitatorKnown(t *testing.T) {
	ctx, store := setup(t)

	known := "0xfacknown00000000000000000000000000000001"
	_, err := store.Pool().Exec(ctx, `
		INSERT INTO facilitator_allowlist (chain, address, source, since_version)
		VALUES ('base', $1, 'manual', 1)`, known)
	require.NoError(t, err)

	mk := func(txHash, facilitator string, logIndex uint32) x402.Payment {
		return x402.Payment{
			Chain: x402.ChainBase, TxHash: txHash, LogIndex: logIndex,
			BlockNumber: 100, BlockTimestamp: time.Unix(1_700_000_000, 0).UTC(),
			Source: "base-collector", Protocol: "x402",
			Facilitator: facilitator, Payer: "0xpay", Payee: "0xrec",
			Asset: "USDC", TokenAddress: strings.ToLower(x402.USDCProxyBase.Hex()),
			AmountRaw: big.NewInt(1_000_000), AssetUSDAtTime: decimalOne(),
			AuthNonce: []byte{0x01}, MethodSelector: []byte{0xe3, 0xee, 0x16, 0x0e},
			CalledContract: strings.ToLower(x402.USDCProxyBase.Hex()),
			TxType: 2, TxNonce: 7, GasUsed: 50_000,
			EffectiveGasPrice: big.NewInt(1_000_000_000), GasCostWei: big.NewInt(50_000_000_000_000),
			SettlementKind: "transfer", TokenDecimals: 6, TokenSymbol: "USDC",
		}
	}

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{
		mk("0xknown", known, 1),
		mk("0xunknown", "0xsomerandomfacilitator0000000000000000001", 1),
	}, 100))

	knownLabel := func(txHash string) bool {
		var fk bool
		require.NoError(t, store.Pool().QueryRow(ctx,
			`SELECT facilitator_known FROM payment_x402_v1 WHERE tx_hash = $1`, txHash).Scan(&fk))
		return fk
	}
	require.True(t, knownLabel("0xknown"), "allowlisted facilitator → known")
	require.False(t, knownLabel("0xunknown"), "unlisted facilitator → unknown (discovery frontier)")
}
