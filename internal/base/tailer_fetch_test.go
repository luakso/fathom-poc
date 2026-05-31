package base

import (
	"context"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/x402"
)

// fakeRPC implements Client with a static fixture. Call counters are guarded
// by mu because FetchRange fetches blocks and receipts concurrently.
type fakeRPC struct {
	tip      uint64
	logs     []types.Log
	blocks   map[uint64]*types.Block
	receipts map[uint64][]*types.Receipt

	mu           sync.Mutex
	filterCalls  int
	blockCalls   int
	receiptCalls int
}

func (f *fakeRPC) BlockNumber(_ context.Context) (uint64, error) { return f.tip, nil }
func (f *fakeRPC) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	f.mu.Lock()
	f.filterCalls++
	f.mu.Unlock()
	return f.logs, nil
}

func (f *fakeRPC) BlockByNumber(_ context.Context, n uint64) (*types.Block, error) {
	f.mu.Lock()
	f.blockCalls++
	f.mu.Unlock()
	return f.blocks[n], nil
}

func (f *fakeRPC) BlockReceipts(_ context.Context, n uint64) ([]*types.Receipt, error) {
	f.mu.Lock()
	f.receiptCalls++
	f.mu.Unlock()
	return f.receipts[n], nil
}
func (f *fakeRPC) Close() {}

// buildClassicTx builds an x402-shaped tx with classic-sig transferWithAuthorization input.
// The tx is unsigned (no key available in tests); tx.Hash() is deterministic from the fields.
// buildClassicTx builds a tx whose calldata carries the ALLOWED classic-sig
// transferWithAuthorization selector. nonce varies the resulting tx hash so
// callers can place distinct txs in distinct blocks.
func buildClassicTx(t *testing.T, nonce uint64) *types.Transaction {
	t.Helper()
	to := x402.USDCProxyBase
	// SighashTransferWithAuthV = 0xe3ee160e (ALLOWED)
	data := append([]byte{0xe3, 0xee, 0x16, 0x0e}, make([]byte, 32)...)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(8453),
		Nonce:     nonce,
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

	tx := buildClassicTx(t, 7)
	header := &types.Header{Number: big.NewInt(100), Time: 1_700_000_000}
	block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})

	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	// Real USDC EIP-3009 order: AuthorizationUsed (index 0) then Transfer (index 1).
	// AuthorizationUsed has 3 topics (sig, authorizer, indexed nonce).
	authLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.AuthorizationUsedTopic, common.HexToHash(payer), common.BytesToHash(make32(0xaa))},
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       0,
	}
	transferLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.TransferTopic, common.HexToHash(payer), common.HexToHash(payee)},
		Data:        encodeU64(1_000_000),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       1,
	}
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: 1, Logs: []*types.Log{&authLog, &transferLog}}

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
	require.Equal(t, 1, rpc.filterCalls, "one eth_getLogs call for the range")
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

	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	authLog := types.Log{
		Address: x402.USDCProxyBase,
		Topics: []common.Hash{
			x402.AuthorizationUsedTopic,
			common.HexToHash(payer),
		},
		Data:        make32(0xaa),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       1,
	}
	// A valid companion Transfer log is present in the receipt: the row is
	// otherwise assemblable, so the ONLY reason it must drop is the denied
	// sighash. Without this companion the test would pass even if the gate
	// were broken (Assemble would drop the row for "no companion Transfer").
	transferLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.TransferTopic, common.HexToHash(payer), common.HexToHash(payee)},
		Data:        encodeU64(1_000_000),
		BlockNumber: 100,
		TxHash:      tx.Hash(),
		Index:       0,
	}
	rpc := &fakeRPC{
		tip:      110,
		logs:     []types.Log{authLog},
		blocks:   map[uint64]*types.Block{100: block},
		receipts: map[uint64][]*types.Receipt{100: {{TxHash: tx.Hash(), Logs: []*types.Log{&transferLog, &authLog}}}},
	}
	payments, _, err := FetchRange(ctx, rpc, 100, 100, 4)
	require.NoError(t, err)
	require.Empty(t, payments, "receiveWithAuthorization must be filtered out")
}

// classicBlockFixture builds a single block at blockNum containing one allowed
// transferWithAuthorization tx, the AuthorizationUsed log returned by
// eth_getLogs, and the receipt (Transfer + AuthorizationUsed companions). The
// nonce keeps each block's tx hash distinct.
func classicBlockFixture(t *testing.T, blockNum, nonce uint64) (*types.Block, types.Log, *types.Receipt) {
	t.Helper()
	tx := buildClassicTx(t, nonce)
	header := &types.Header{Number: new(big.Int).SetUint64(blockNum), Time: 1_700_000_000}
	block := types.NewBlockWithHeader(header).WithBody(types.Body{Transactions: types.Transactions{tx}})

	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	// Real USDC EIP-3009 order: AuthorizationUsed (index 0) then Transfer (index 1).
	// AuthorizationUsed has 3 topics (sig, authorizer, indexed nonce).
	authLog := types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.AuthorizationUsedTopic, common.HexToHash(payer), common.BytesToHash(make32(0xaa))},
		BlockNumber: blockNum,
		TxHash:      tx.Hash(),
		Index:       0,
	}
	transferLog := &types.Log{
		Address:     x402.USDCProxyBase,
		Topics:      []common.Hash{x402.TransferTopic, common.HexToHash(payer), common.HexToHash(payee)},
		Data:        encodeU64(1_000_000),
		BlockNumber: blockNum,
		TxHash:      tx.Hash(),
		Index:       1,
	}
	authLogCopy := authLog
	receipt := &types.Receipt{TxHash: tx.Hash(), Status: 1, Logs: []*types.Log{&authLogCopy, transferLog}}
	return block, authLog, receipt
}

func TestFetchRange_FetchesAcrossBlocksConcurrently(t *testing.T) {
	ctx := context.Background()

	block100, auth100, receipt100 := classicBlockFixture(t, 100, 1)
	block101, auth101, receipt101 := classicBlockFixture(t, 101, 2)

	rpc := &fakeRPC{
		tip:    120,
		logs:   []types.Log{auth100, auth101},
		blocks: map[uint64]*types.Block{100: block100, 101: block101},
		receipts: map[uint64][]*types.Receipt{
			100: {receipt100},
			101: {receipt101},
		},
	}

	payments, maxBlock, err := FetchRange(ctx, rpc, 100, 101, 2)
	require.NoError(t, err)
	require.Len(t, payments, 2, "one payment per block")
	require.Equal(t, uint64(101), maxBlock)
	require.Equal(t, 2, rpc.blockCalls, "both blocks fetched")
	require.Equal(t, 2, rpc.receiptCalls, "receipts fetched for both surviving blocks")
}

func make32(b byte) []byte { out := make([]byte, 32); out[0] = b; return out }
func encodeU64(v uint64) []byte {
	out := make([]byte, 32)
	for i := range 8 {
		out[31-i] = byte(v >> (8 * i))
	}
	return out
}
