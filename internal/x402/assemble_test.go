package x402

import (
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// txFixture builds a Transaction with sensible defaults for the assemble tests.
func txFixture(hash, from, to string, input []byte) Transaction {
	return Transaction{
		Hash:              common.HexToHash(hash),
		BlockNumber:       42,
		From:              common.HexToAddress(from),
		To:                common.HexToAddress(to),
		Input:             input,
		Type:              2,
		Nonce:             7,
		GasUsed:           50_000,
		EffectiveGasPrice: big.NewInt(1_000_000_000),
	}
}

func TestAssemble_SinglePayment_DirectUSDCCall(t *testing.T) {
	tx := txFixture(
		"0xdead",
		"0xfaC1000000000000000000000000000000000001", // facilitator (tx.from)
		USDCProxyBase.Hex(),
		[]byte{0xe3, 0xee, 0x16, 0x0e}, // transferWithAuthorization classic
	)
	tx.Hash = common.HexToHash("0xdead")

	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"

	logs := []Log{
		// Real USDC EIP-3009 order: AuthorizationUsed first (log_index=0)...
		// Both event params are indexed → 3 topics (sig, authorizer, nonce),
		// empty data.
		{
			Address: USDCProxyBase,
			Topics: []common.Hash{
				AuthorizationUsedTopic,
				common.HexToHash(payer),
				common.BytesToHash(bytes32(0xab)), // nonce (indexed)
			},
			BlockNumber: 42,
			TxHash:      tx.Hash,
			LogIndex:    0,
		},
		// ...then the companion Transfer at log_index=1.
		{
			Address: USDCProxyBase,
			Topics: []common.Hash{
				TransferTopic,
				common.HexToHash(payer),
				common.HexToHash(payee),
			},
			// 1_000_000 = 1 USDC
			Data:        make32WithUint64(1_000_000),
			BlockNumber: 42,
			TxHash:      tx.Hash,
			LogIndex:    1,
		},
	}

	block := Block{Number: 42, Timestamp: 1_700_000_000, BaseFeePerGas: big.NewInt(500_000_000)}
	out := Assemble(
		logs,
		map[common.Hash]Transaction{tx.Hash: tx},
		map[common.Hash][]Log{tx.Hash: logs},
		map[uint64]Block{42: block},
	)
	require.Len(t, out, 1)
	p := out[0]
	require.Equal(t, ChainBase, p.Chain)
	require.Equal(t, uint32(0), p.LogIndex) // the AuthorizationUsed log index
	require.Equal(t, "0xfac1000000000000000000000000000000000001", p.Facilitator)
	require.Equal(t, "0xaaaa000000000000000000000000000000000001", p.Payer)
	require.Equal(t, "0xbbbb000000000000000000000000000000000001", p.Payee)
	require.Equal(t, big.NewInt(1_000_000), p.AmountRaw)
	require.Equal(t, "1.000000", p.AmountUSDC.StringFixed(6))
	require.Equal(t, time.Unix(1_700_000_000, 0).UTC(), p.BlockTimestamp)
	require.Equal(t, []byte{0xe3, 0xee, 0x16, 0x0e}, p.MethodSelector)
	require.Equal(t, USDCProxyBase.Hex(), common.HexToAddress(p.CalledContract).Hex()) // round-trip
	require.Equal(t, big.NewInt(500_000_000), p.BaseFeePerGas, "base fee sourced from the block")
}

func TestAssemble_DropsReceiveWithAuthorization(t *testing.T) {
	tx := txFixture(
		"0xfeed",
		"0xfaC1000000000000000000000000000000000001",
		USDCProxyBase.Hex(),
		[]byte{0xef, 0x55, 0xbe, 0xc6}, // receiveWithAuthorization classic — DENY
	)
	tx.Hash = common.HexToHash("0xfeed")

	logs := []Log{
		{
			Address: USDCProxyBase,
			Topics: []common.Hash{
				TransferTopic,
				common.HexToHash("0x000000000000000000000000aaaa000000000000000000000000000000000001"),
				common.HexToHash("0x000000000000000000000000bbbb000000000000000000000000000000000001"),
			},
			Data: make32WithUint64(1_000_000), TxHash: tx.Hash, LogIndex: 0, BlockNumber: 42,
		},
		{
			Address: USDCProxyBase,
			Topics: []common.Hash{
				AuthorizationUsedTopic,
				common.HexToHash("0x000000000000000000000000aaaa000000000000000000000000000000000001"),
			},
			Data: bytes32(0xab), TxHash: tx.Hash, LogIndex: 1, BlockNumber: 42,
		},
	}
	out := Assemble(
		logs,
		map[common.Hash]Transaction{tx.Hash: tx},
		map[common.Hash][]Log{tx.Hash: logs},
		map[uint64]Block{42: {Number: 42, Timestamp: 1_700_000_000}},
	)
	require.Empty(t, out, "receiveWithAuthorization tx must be dropped at the filter")
}

func TestAssemble_MulticallProducesMultipleRows(t *testing.T) {
	tx := txFixture(
		"0xbeef",
		"0xfaC1000000000000000000000000000000000001",
		Multicall3.Hex(),
		[]byte{0x82, 0xad, 0x56, 0xcb}, // aggregate3
	)
	tx.Hash = common.HexToHash("0xbeef")

	payerA := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payeeA := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	payerB := "0x000000000000000000000000cccc000000000000000000000000000000000001"
	payeeB := "0x000000000000000000000000dddd000000000000000000000000000000000001"

	// Real USDC EIP-3009 order per payment: AuthorizationUsed then Transfer.
	// Two interleaved payments: [AUTH_A@0, XFER_A@1, AUTH_B@2, XFER_B@3].
	logs := []Log{
		{Address: USDCProxyBase, Topics: []common.Hash{AuthorizationUsedTopic, common.HexToHash(payerA), common.BytesToHash(bytes32(0xaa))}, TxHash: tx.Hash, LogIndex: 0, BlockNumber: 42},
		{Address: USDCProxyBase, Topics: []common.Hash{TransferTopic, common.HexToHash(payerA), common.HexToHash(payeeA)}, Data: make32WithUint64(100), TxHash: tx.Hash, LogIndex: 1, BlockNumber: 42},
		{Address: USDCProxyBase, Topics: []common.Hash{AuthorizationUsedTopic, common.HexToHash(payerB), common.BytesToHash(bytes32(0xbb))}, TxHash: tx.Hash, LogIndex: 2, BlockNumber: 42},
		{Address: USDCProxyBase, Topics: []common.Hash{TransferTopic, common.HexToHash(payerB), common.HexToHash(payeeB)}, Data: make32WithUint64(500), TxHash: tx.Hash, LogIndex: 3, BlockNumber: 42},
	}
	out := Assemble(
		logs,
		map[common.Hash]Transaction{tx.Hash: tx},
		map[common.Hash][]Log{tx.Hash: logs},
		map[uint64]Block{42: {Number: 42, Timestamp: 1_700_000_000}},
	)
	require.Len(t, out, 2)

	require.Equal(t, uint32(0), out[0].LogIndex)
	require.Equal(t, "0xaaaa000000000000000000000000000000000001", out[0].Payer)
	require.Equal(t, big.NewInt(100), out[0].AmountRaw)

	require.Equal(t, uint32(2), out[1].LogIndex)
	require.Equal(t, "0xcccc000000000000000000000000000000000001", out[1].Payer)
	require.Equal(t, big.NewInt(500), out[1].AmountRaw)
}

func TestAssemble_SkipsAuthMissingCompanionTransfer(t *testing.T) {
	tx := txFixture(
		"0xfeed",
		"0xfaC1000000000000000000000000000000000001",
		USDCProxyBase.Hex(),
		[]byte{0xe3, 0xee, 0x16, 0x0e},
	)
	tx.Hash = common.HexToHash("0xfeed")
	logs := []Log{
		{
			Address: USDCProxyBase,
			Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x000000000000000000000000aaaa000000000000000000000000000000000001")},
			Data:    bytes32(0xaa), TxHash: tx.Hash, LogIndex: 0, BlockNumber: 42,
		},
	}
	out := Assemble(
		logs,
		map[common.Hash]Transaction{tx.Hash: tx},
		map[common.Hash][]Log{tx.Hash: logs},
		map[uint64]Block{42: {Number: 42, Timestamp: 1_700_000_000}},
	)
	require.Empty(t, out, "auth without a companion Transfer must be skipped, not exploded")
}

// helpers
func bytes32(b byte) []byte {
	out := make([]byte, 32)
	out[0] = b
	return out
}

func make32WithUint64(v uint64) []byte {
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		out[31-i] = byte(v >> (8 * i))
	}
	return out
}
