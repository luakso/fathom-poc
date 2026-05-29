package base

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/x402"
)

// fakeRPC implements Client with a static fixture.
type fakeRPC struct {
	tip         uint64
	logs        []types.Log
	blocks      map[uint64]*types.Block
	receipts    map[uint64][]*types.Receipt
	filterCalls int
}

func (f *fakeRPC) BlockNumber(_ context.Context) (uint64, error) { return f.tip, nil }
func (f *fakeRPC) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	f.filterCalls++
	return f.logs, nil
}

func (f *fakeRPC) BlockByNumber(_ context.Context, n uint64) (*types.Block, error) {
	return f.blocks[n], nil
}

func (f *fakeRPC) BlockReceipts(_ context.Context, n uint64) ([]*types.Receipt, error) {
	return f.receipts[n], nil
}
func (f *fakeRPC) Close() {}

// buildClassicTx builds an x402-shaped tx with classic-sig transferWithAuthorization input.
// The tx is unsigned (no key available in tests); tx.Hash() is deterministic from the fields.
func buildClassicTx(t *testing.T, _ common.Hash, _ common.Address) *types.Transaction {
	t.Helper()
	to := x402.USDCProxyBase
	// SighashTransferWithAuthV = 0xe3ee160e (ALLOWED)
	data := append([]byte{0xe3, 0xee, 0x16, 0x0e}, make([]byte, 32)...)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(8453),
		Nonce:     7,
		GasTipCap: big.NewInt(0),
		GasFeeCap: big.NewInt(1_000_000_000),
		Gas:       50_000,
		To:        &to,
		Data:      data,
	})
	return tx
}

func TestFetchRange_FilterCutsByAddressTopic(t *testing.T) {
	ctx := context.Background()

	tx := buildClassicTx(t, common.HexToHash("0xdead"), common.HexToAddress("0xfac"))
	header := &types.Header{Number: big.NewInt(100), Time: 1_700_000_000}
	block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})

	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	authLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.AuthorizationUsedTopic, common.HexToHash(payer)},
		Data:        make32(0xaa),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       1,
	}
	transferLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.TransferTopic, common.HexToHash(payer), common.HexToHash(payee)},
		Data:        encodeU64(1_000_000),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       0,
	}
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: 1, Logs: []*types.Log{&transferLog, &authLog}}

	rpc := &fakeRPC{
		tip:      120,
		logs:     []types.Log{authLog},
		blocks:   map[uint64]*types.Block{100: block},
		receipts: map[uint64][]*types.Receipt{100: {receipt}},
	}

	payments, maxBlock, err := FetchRange(ctx, rpc, 100, 100, 8)
	require.NoError(t, err)
	require.Len(t, payments, 1)
	require.Equal(t, uint64(100), maxBlock)
	require.Equal(t, big.NewInt(1_000_000), payments[0].AmountRaw)
}

func TestFetchRange_NoLogsAdvancesNothing(t *testing.T) {
	ctx := context.Background()
	rpc := &fakeRPC{tip: 100, logs: nil, blocks: map[uint64]*types.Block{}, receipts: map[uint64][]*types.Receipt{}}
	payments, maxBlock, err := FetchRange(ctx, rpc, 50, 80, 8)
	require.NoError(t, err)
	require.Empty(t, payments)
	require.Equal(t, uint64(80), maxBlock, "no logs in range means the whole range is cleared; cursor advances to range end")
}

func TestFetchRange_DropsRowFromDeniedSighash(t *testing.T) {
	ctx := context.Background()

	to := x402.USDCProxyBase
	// SighashReceiveWithAuthV = 0xef55bec6 (DENIED)
	data := append([]byte{0xef, 0x55, 0xbe, 0xc6}, make([]byte, 32)...)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID: big.NewInt(8453), Nonce: 1, GasTipCap: big.NewInt(0),
		GasFeeCap: big.NewInt(1), Gas: 50_000, To: &to, Data: data,
	})
	header := &types.Header{Number: big.NewInt(100), Time: 1_700_000_000}
	block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})

	authLog := types.Log{
		Address: x402.USDCProxyBase,
		Topics: []common.Hash{
			x402.AuthorizationUsedTopic,
			common.HexToHash("0x000000000000000000000000aaaa000000000000000000000000000000000001"),
		},
		Data:        make32(0xaa),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       1,
	}
	rpc := &fakeRPC{
		tip:      110,
		logs:     []types.Log{authLog},
		blocks:   map[uint64]*types.Block{100: block},
		receipts: map[uint64][]*types.Receipt{100: {{TxHash: tx.Hash(), Logs: []*types.Log{&authLog}}}},
	}
	payments, _, err := FetchRange(ctx, rpc, 100, 100, 4)
	require.NoError(t, err)
	require.Empty(t, payments, "receiveWithAuthorization must be filtered out")
}

func make32(b byte) []byte { out := make([]byte, 32); out[0] = b; return out }
func encodeU64(v uint64) []byte {
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		out[31-i] = byte(v >> (8 * i))
	}
	return out
}
