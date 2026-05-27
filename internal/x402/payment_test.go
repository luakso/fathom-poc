package x402

import (
	"math/big"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestUSDCFromRaw_KnownValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw     *big.Int
		wantUSD string // decimal string for exact comparison
	}{
		{big.NewInt(1_000_000), "1.000000"}, // 1.00 USDC
		{big.NewInt(1), "0.000001"},         // 1 micro-USDC (dust)
		{big.NewInt(0), "0.000000"},         // zero
		{big.NewInt(123_456_789), "123.456789"},
	}
	for _, c := range cases {
		t.Run(c.wantUSD, func(t *testing.T) {
			t.Parallel()
			got := USDCFromRaw(c.raw)
			require.Equal(t, c.wantUSD, got.StringFixed(USDCDecimals))
		})
	}
}

func TestPayment_ZeroValue(t *testing.T) {
	t.Parallel()
	// A zero-value Payment must be a safe, distinguishable empty state — every
	// pointer/slice field nil, every string empty, every time the zero time.
	// We rely on this in the Assemble step to detect "no companion Transfer".
	var p Payment
	require.Empty(t, p.TxHash)
	require.Equal(t, time.Time{}, p.BlockTimestamp)
	require.Nil(t, p.AmountRaw)
}

func TestPayment_FieldsAccept(t *testing.T) {
	t.Parallel()
	// Smoke test that the struct accepts the kinds of values we'll throw at it
	// from a real decode. No behavior here — this is a shape lock.
	now := time.Now().UTC()
	p := Payment{
		Chain:             ChainBase,
		TxHash:            "0xabc",
		LogIndex:          3,
		BlockNumber:       40_222_720,
		BlockTimestamp:    now,
		Source:            "base-collector",
		Protocol:          "x402",
		Facilitator:       "0xfac",
		Payer:             "0xpay",
		Payee:             "0xrec",
		PayeeServiceID:    nil,
		Asset:             "USDC",
		TokenAddress:      USDCProxyBase.Hex(),
		AmountRaw:         big.NewInt(1_000_000),
		AmountUSDC:        decimal.NewFromInt(1),
		AssetUSDAtTime:    decimal.NewFromInt(1),
		AuthNonce:         []byte{0x01, 0x02},
		MethodSelector:    []byte{0xe3, 0xee, 0x16, 0x0e},
		CalledContract:    USDCProxyBase.Hex(),
		TxType:            2,
		TxNonce:           42,
		GasUsed:           50_000,
		EffectiveGasPrice: big.NewInt(1_000_000_000),
		GasCostWei:        big.NewInt(50_000_000_000_000),
		BaseFeePerGas:     big.NewInt(500_000_000),
	}
	require.Equal(t, uint32(3), p.LogIndex)
	require.NotNil(t, p.AmountRaw)
}
