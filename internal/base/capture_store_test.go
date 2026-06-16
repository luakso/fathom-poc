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

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{p}, nil, 100))

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
		&inputCalldata, &blockHash, &txIndex, &tokenDecimals, &tokenSymbol, &payerAccountType,
	))

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

// TestMigration_L1CaptureColumns asserts the 00012 migration added the L1-fee /
// value / gas-limit capture columns to payments and left both read views intact
// (recreated through the column add).
func TestMigration_L1CaptureColumns(t *testing.T) {
	ctx, store := setup(t)

	wantCols := []string{"l1_fee", "l1_gas_used", "l1_gas_price", "tx_value", "gas_limit"}
	for _, col := range wantCols {
		var exists bool
		require.NoError(t, store.Pool().QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'payments' AND column_name = $1
			)`, col).Scan(&exists))
		require.True(t, exists, "payments.%s must exist after migration 00012", col)
	}

	// Both p.*-based views survive the column add (recreated in the migration).
	_, err := store.Pool().Exec(ctx, `SELECT facilitator_known FROM payment_x402_v1 WHERE false`)
	require.NoError(t, err, "payment_x402_v1 must still be selectable")
	_, err = store.Pool().Exec(ctx, `SELECT attribution FROM payment_classified_v1 WHERE false`)
	require.NoError(t, err, "payment_classified_v1 must still be selectable")
}

// decimalOne is a tiny local helper for the value 1 (avoids importing shopspring
// at every call site).
func decimalOne() decimal.Decimal { return decimal.NewFromInt(1) }

// TestStore_L1CaptureFields_RoundTrip proves the Plan 2 capture columns (L1 fee
// trio, tx_value, gas_limit) persist through the COPY → INSERT path, including a
// row that leaves the nullable L1 fields nil (→ SQL NULL).
func TestStore_L1CaptureFields_RoundTrip(t *testing.T) {
	ctx, store := setup(t)

	mk := func(txHash string) x402.Payment {
		return x402.Payment{
			Chain: x402.ChainBase, TxHash: txHash, LogIndex: 1,
			BlockNumber: 100, BlockTimestamp: time.Unix(1_700_000_000, 0).UTC(),
			Source: "base-collector", Protocol: "x402",
			Facilitator: "0xfac", Payer: "0xpay", Payee: "0xrec",
			Asset: "USDC", TokenAddress: strings.ToLower(x402.USDCProxyBase.Hex()),
			AmountRaw: big.NewInt(1_000_000), AssetUSDAtTime: decimalOne(),
			AuthNonce: []byte{0x01}, MethodSelector: []byte{0xe3, 0xee, 0x16, 0x0e},
			CalledContract: strings.ToLower(x402.USDCProxyBase.Hex()),
			TxType:         2, TxNonce: 7, GasUsed: 50_000,
			EffectiveGasPrice: big.NewInt(1_000_000_000), GasCostWei: big.NewInt(50_000_000_000_000),
			SettlementKind: "transfer", TokenDecimals: 6, TokenSymbol: "USDC",
		}
	}

	withL1 := mk("0xl1set")
	withL1.L1Fee = big.NewInt(12_345)
	withL1.L1GasUsed = big.NewInt(1_600)
	withL1.L1GasPrice = big.NewInt(7)
	withL1.TxValue = big.NewInt(0)
	withL1.GasLimit = 120_000

	nilL1 := mk("0xl1nil") // L1Fee/L1GasUsed/L1GasPrice/TxValue nil; GasLimit 0

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{withL1, nilL1}, nil, 100))

	// Row with values: NUMERIC columns read back via ::text for an exact compare.
	var (
		l1Fee, l1GasUsed, l1GasPrice, txValue string
		gasLimit                              int64
	)
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT l1_fee::text, l1_gas_used::text, l1_gas_price::text, tx_value::text, gas_limit
		FROM payments WHERE tx_hash = $1`, "0xl1set").Scan(
		&l1Fee, &l1GasUsed, &l1GasPrice, &txValue, &gasLimit,
	))
	require.Equal(t, "12345", l1Fee)
	require.Equal(t, "1600", l1GasUsed)
	require.Equal(t, "7", l1GasPrice)
	require.Equal(t, "0", txValue)
	require.Equal(t, int64(120_000), gasLimit)

	// Row with nil L1 fields stores SQL NULL for ALL four nullable numerics;
	// gas_limit 0 stores 0 (not NULL). Each nullable column is asserted
	// individually so a regression in only one is caught at the column level.
	var (
		l1FeeNil, l1GasUsedNil, l1GasPriceNil, txValueNil *string
		gasLimitNil                                       *int64
	)
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT l1_fee::text, l1_gas_used::text, l1_gas_price::text, tx_value::text, gas_limit
		FROM payments WHERE tx_hash = $1`, "0xl1nil").Scan(
		&l1FeeNil, &l1GasUsedNil, &l1GasPriceNil, &txValueNil, &gasLimitNil,
	))
	require.Nil(t, l1FeeNil, "nil L1Fee → SQL NULL")
	require.Nil(t, l1GasUsedNil, "nil L1GasUsed → SQL NULL")
	require.Nil(t, l1GasPriceNil, "nil L1GasPrice → SQL NULL")
	require.Nil(t, txValueNil, "nil TxValue → SQL NULL")
	require.NotNil(t, gasLimitNil)
	require.Equal(t, int64(0), *gasLimitNil)
}

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
			TxType:         2, TxNonce: 7, GasUsed: 50_000,
			EffectiveGasPrice: big.NewInt(1_000_000_000), GasCostWei: big.NewInt(50_000_000_000_000),
			SettlementKind: "transfer", TokenDecimals: 6, TokenSymbol: "USDC",
		}
	}

	require.NoError(t, store.InsertBatch(ctx, []x402.Payment{
		mk("0xknown", known, 1),
		mk("0xunknown", "0xsomerandomfacilitator0000000000000000001", 1),
	}, nil, 100))

	knownLabel := func(txHash string) bool {
		var fk bool
		require.NoError(t, store.Pool().QueryRow(ctx,
			`SELECT facilitator_known FROM payment_x402_v1 WHERE tx_hash = $1`, txHash).Scan(&fk))
		return fk
	}
	require.True(t, knownLabel("0xknown"), "allowlisted facilitator → known")
	require.False(t, knownLabel("0xunknown"), "unlisted facilitator → unknown (discovery frontier)")
}

// TestMigration_AuthorizationCancellations asserts 00013 created the table and
// authorization_cancellation_v1 exposes facilitator_known.
func TestMigration_AuthorizationCancellations(t *testing.T) {
	ctx, store := setup(t)

	var exists bool
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'authorization_cancellations')`).Scan(&exists))
	require.True(t, exists, "authorization_cancellations table must exist after migration 00013")

	_, err := store.Pool().Exec(ctx, `SELECT facilitator_known FROM authorization_cancellation_v1 WHERE false`)
	require.NoError(t, err, "authorization_cancellation_v1 must expose facilitator_known")
}

// TestStore_Cancellations_RoundTrip proves cancellations persist through
// InsertBatch (in the same transaction as payments + cursor) and that re-insert
// is idempotent.
func TestStore_Cancellations_RoundTrip(t *testing.T) {
	ctx, store := setup(t)

	nonce := make([]byte, 32)
	nonce[31] = 0xab
	c := x402.Cancellation{
		Chain: x402.ChainBase, TxHash: "0xcancel", LogIndex: 4,
		Authorizer: "0xpayer", Nonce: nonce,
		BlockNumber: 100, BlockTime: time.Unix(1_700_000_000, 0).UTC(),
		TransactionFrom: "0xfac",
	}

	require.NoError(t, store.InsertBatch(ctx, nil, []x402.Cancellation{c}, 100))

	var (
		gotAuthorizer, gotFrom string
		gotNonce               []byte
		gotBlock               int64
	)
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT authorizer, transaction_from, nonce, block_number
		FROM authorization_cancellations WHERE tx_hash = $1 AND log_index = $2`,
		"0xcancel", 4).Scan(&gotAuthorizer, &gotFrom, &gotNonce, &gotBlock))
	require.Equal(t, "0xpayer", gotAuthorizer)
	require.Equal(t, "0xfac", gotFrom)
	require.Equal(t, nonce, gotNonce)
	require.Equal(t, int64(100), gotBlock)

	// Idempotent re-insert: no error, still exactly one row.
	require.NoError(t, store.InsertBatch(ctx, nil, []x402.Cancellation{c}, 100))
	var count int
	require.NoError(t, store.Pool().QueryRow(ctx, `
		SELECT count(*) FROM authorization_cancellations WHERE tx_hash = $1`, "0xcancel").Scan(&count))
	require.Equal(t, 1, count, "re-insert must not duplicate")
}
