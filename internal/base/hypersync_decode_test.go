package base_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/x402"
)

func TestConvertHyperSyncLog(t *testing.T) {
	t.Parallel()
	hl := base.HyperSyncLog{
		Address:     "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Topics:      []string{x402.TransferTopic.Hex(), "0x0000000000000000000000000000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000000000000000000000000000002"},
		Data:        "0x00000000000000000000000000000000000000000000000000000000000f4240",
		BlockNumber: 100,
		TxHash:      "0xabc",
		TxIndex:     3,
		LogIndex:    7,
	}
	got, err := base.ConvertLog(hl)
	require.NoError(t, err)
	require.Equal(t, x402.USDCProxyBase, got.Address)
	require.Len(t, got.Topics, 3)
	require.Equal(t, x402.TransferTopic, got.Topics[0])
	require.Equal(t, 32, len(got.Data))
	require.Equal(t, uint64(100), got.BlockNumber)
	require.Equal(t, uint32(7), got.LogIndex)
}

func TestConvertHyperSyncTransaction(t *testing.T) {
	t.Parallel()
	htx := base.HyperSyncTransaction{
		Hash:                 "0xdead",
		BlockNumber:          42,
		From:                 "0xfac1000000000000000000000000000000000001",
		To:                   "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Input:                "0xe3ee160edeadbeef",
		Type:                 2,
		Nonce:                "0x7",        // 7
		GasUsed:              "0xc350",     // 50_000
		EffectiveGasPrice:    "0x3b9aca00", // 1_000_000_000
		MaxFeePerGas:         "0x77359400", // 2_000_000_000
		MaxPriorityFeePerGas: "0x16e360",   // 1_500_000
	}
	got, err := base.ConvertTransaction(htx)
	require.NoError(t, err)
	require.Equal(t, common.HexToHash("0xdead"), got.Hash)
	require.Equal(t, uint64(42), got.BlockNumber)
	require.Equal(t, x402.USDCProxyBase, got.To)
	require.Equal(t, []byte{0xe3, 0xee, 0x16, 0x0e, 0xde, 0xad, 0xbe, 0xef}, got.Input)
	require.Equal(t, big.NewInt(1_000_000_000), got.EffectiveGasPrice)
	require.Equal(t, big.NewInt(2_000_000_000), got.MaxFeePerGas)
	require.Equal(t, big.NewInt(1_500_000), got.MaxPriorityFeePerGas)
	require.Equal(t, uint64(7), got.Nonce)
	require.Equal(t, uint64(50_000), got.GasUsed)
}

// TestConvertHyperSyncTransaction_LegacyNilFeeCaps locks in that a legacy/
// EIP-2930 tx — which carries no EIP-1559 fee market — leaves MaxFeePerGas and
// MaxPriorityFeePerGas nil (→ SQL NULL) rather than coercing empty to 0.
func TestConvertHyperSyncTransaction_LegacyNilFeeCaps(t *testing.T) {
	t.Parallel()
	htx := base.HyperSyncTransaction{
		Hash:              "0xdead",
		From:              "0xfac1000000000000000000000000000000000001",
		To:                "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913",
		Input:             "0xe3ee160e",
		Type:              0,
		Nonce:             "0x1",
		GasUsed:           "0xc350",
		EffectiveGasPrice: "0x3b9aca00",
		// MaxFeePerGas / MaxPriorityFeePerGas absent → empty strings.
	}
	got, err := base.ConvertTransaction(htx)
	require.NoError(t, err)
	require.Nil(t, got.MaxFeePerGas, "legacy tx has no max_fee_per_gas")
	require.Nil(t, got.MaxPriorityFeePerGas, "legacy tx has no max_priority_fee_per_gas")
}

func TestConvertHyperSyncBlock(t *testing.T) {
	t.Parallel()
	got, err := base.ConvertBlock(base.HyperSyncBlock{
		Number: 100, Timestamp: "0x6553f100", Hash: "0xabc", // 1_700_000_000
		BaseFeePerGas: "0x1dcd6500", // 500_000_000
	})
	require.NoError(t, err)
	require.Equal(t, uint64(100), got.Number)
	require.Equal(t, uint64(1_700_000_000), got.Timestamp)
	require.Equal(t, big.NewInt(500_000_000), got.BaseFeePerGas)
}

func TestConvertHyperSyncBlock_LegacyHasNilBaseFee(t *testing.T) {
	t.Parallel()
	got, err := base.ConvertBlock(base.HyperSyncBlock{Number: 100, Timestamp: "0x6553f100", Hash: "0xabc", BaseFeePerGas: ""})
	require.NoError(t, err)
	require.Nil(t, got.BaseFeePerGas)
}

func TestParseHexInt_RejectsBadInput(t *testing.T) {
	t.Parallel()
	_, err := base.ParseHexInt("not-hex")
	require.Error(t, err)
}
