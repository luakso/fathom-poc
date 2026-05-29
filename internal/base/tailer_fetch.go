package base

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/lukostrobl/fathom/internal/x402"
)

// FetchRange runs the live-tail block-batching algorithm over the closed
// block range [fromBlock, toBlock]. It returns the assembled []Payment slice
// and the max block actually queried (= toBlock unless an error happened
// earlier). Concurrency bounds the number of simultaneous RPC round-trips;
// values < 1 are clamped to 1.
//
// Algorithm (spec §8):
//  1. eth_getLogs filtered to USDC address + AuthorizationUsed topic.
//  2. eth_getBlockByNumber (with full txs) for every unique block in the logs.
//  3. Client-side sighash filter via x402.KeepAuthorizationUsed — drops any log
//     whose parent tx carries a denied or unknown selector.
//  4. eth_getBlockReceipts for each block that has at least one surviving tx.
//  5. x402.Assemble pairs companions, decodes, and builds Payment rows.
func FetchRange(ctx context.Context, c Client, fromBlock, toBlock uint64, concurrency int64) ([]x402.Payment, uint64, error) {
	if concurrency < 1 {
		concurrency = 1
	}

	// Step 1: fetch logs.
	filter := ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(fromBlock),
		ToBlock:   new(big.Int).SetUint64(toBlock),
		Addresses: []common.Address{x402.USDCProxyBase},
		Topics:    [][]common.Hash{{x402.AuthorizationUsedTopic}},
	}

	var logs []types.Log
	if err := WithRateLimitBackoff(ctx, func() error {
		var fetchErr error
		logs, fetchErr = c.FilterLogs(ctx, filter)
		return fetchErr
	}, DefaultBackoff); err != nil {
		return nil, 0, fmt.Errorf("filter logs %d-%d: %w", fromBlock, toBlock, err)
	}
	if len(logs) == 0 {
		return nil, toBlock, nil
	}

	// Step 2: fetch blocks (full transactions) for each unique block number.
	blockNums := uniqueBlockNumbers(logs)
	blocksByNumber, err := fetchBlocks(ctx, c, blockNums, concurrency)
	if err != nil {
		return nil, 0, err
	}

	// Step 3: sighash filter — keep only logs whose parent tx passes KeepAuthorizationUsed.
	survivingBlocks := map[uint64]struct{}{}
	for i := range logs {
		lg := logs[i]
		blk, ok := blocksByNumber[lg.BlockNumber]
		if !ok {
			continue
		}
		tx := txByHash(blk, lg.TxHash)
		if tx == nil {
			continue
		}
		xl := x402.Log{
			Address: lg.Address,
			Topics:  lg.Topics,
		}
		if x402.KeepAuthorizationUsed(xl, tx.Data()) {
			survivingBlocks[lg.BlockNumber] = struct{}{}
		}
	}
	if len(survivingBlocks) == 0 {
		return nil, toBlock, nil
	}

	// Step 4: fetch receipts for surviving blocks only.
	survivingNums := make([]uint64, 0, len(survivingBlocks))
	for n := range survivingBlocks {
		survivingNums = append(survivingNums, n)
	}
	sort.Slice(survivingNums, func(i, j int) bool { return survivingNums[i] < survivingNums[j] })
	receiptsByBlock, err := fetchBlockReceipts(ctx, c, survivingNums, concurrency)
	if err != nil {
		return nil, 0, err
	}

	// Step 5: build x402 input maps and call Assemble.
	xLogs := make([]x402.Log, 0, len(logs))
	txByHashMap := map[common.Hash]x402.Transaction{}
	receiptByHashMap := map[common.Hash][]x402.Log{}
	blockByNumberMap := map[uint64]x402.Block{}

	for i := range logs {
		lg := logs[i]
		blk, ok := blocksByNumber[lg.BlockNumber]
		if !ok {
			continue
		}
		tx := txByHash(blk, lg.TxHash)
		if tx == nil {
			continue
		}

		xLog := x402.Log{
			Address:     lg.Address,
			Topics:      lg.Topics,
			Data:        lg.Data,
			BlockNumber: lg.BlockNumber,
			TxHash:      lg.TxHash,
			TxIndex:     uint32(lg.TxIndex), //nolint:gosec // tx index fits uint32
			LogIndex:    uint32(lg.Index),   //nolint:gosec // log index fits uint32
		}
		xLogs = append(xLogs, xLog)

		if _, seen := txByHashMap[lg.TxHash]; !seen {
			from, _ := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
			var toAddr common.Address
			if tx.To() != nil {
				toAddr = *tx.To()
			}
			// Use GasFeeCap as a best-effort effective gas price before the
			// receipt is consulted; receiptByHashMap population below updates
			// it to the actual EffectiveGasPrice from the receipt.
			txByHashMap[lg.TxHash] = x402.Transaction{
				Hash:              lg.TxHash,
				BlockNumber:       lg.BlockNumber,
				From:              from,
				To:                toAddr,
				Input:             tx.Data(),
				Type:              tx.Type(),
				Nonce:             tx.Nonce(),
				GasUsed:           receiptGasUsed(receiptsByBlock[lg.BlockNumber], lg.TxHash),
				EffectiveGasPrice: tx.GasFeeCap(),
				BaseFeePerGas:     blk.BaseFee(),
			}
		}

		if _, seen := receiptByHashMap[lg.TxHash]; !seen {
			rs := receiptsByBlock[lg.BlockNumber]
			for _, r := range rs {
				if r.TxHash != lg.TxHash {
					continue
				}
				logsForTx := make([]x402.Log, 0, len(r.Logs))
				for _, rl := range r.Logs {
					logsForTx = append(logsForTx, x402.Log{
						Address:     rl.Address,
						Topics:      rl.Topics,
						Data:        rl.Data,
						BlockNumber: rl.BlockNumber,
						TxHash:      rl.TxHash,
						TxIndex:     uint32(rl.TxIndex), //nolint:gosec // tx index fits uint32
						LogIndex:    uint32(rl.Index),   //nolint:gosec // log index fits uint32
					})
				}
				receiptByHashMap[lg.TxHash] = logsForTx
				// Update EffectiveGasPrice from the actual receipt value.
				if egp := receiptEffectiveGasPrice(r); egp != nil {
					entry := txByHashMap[lg.TxHash]
					entry.EffectiveGasPrice = egp
					txByHashMap[lg.TxHash] = entry
				}
				break
			}
		}

		if _, seen := blockByNumberMap[lg.BlockNumber]; !seen {
			blockByNumberMap[lg.BlockNumber] = x402.Block{
				Number:    lg.BlockNumber,
				Timestamp: blk.Time(),
				Hash:      blk.Hash(),
			}
		}
	}

	return x402.Assemble(xLogs, txByHashMap, receiptByHashMap, blockByNumberMap), toBlock, nil
}

// uniqueBlockNumbers returns the unique block numbers from logs, sorted ascending.
func uniqueBlockNumbers(logs []types.Log) []uint64 {
	seen := map[uint64]struct{}{}
	out := make([]uint64, 0, len(logs))
	for i := range logs {
		n := logs[i].BlockNumber
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// fetchBlocks fetches the given block numbers concurrently, returning a map
// keyed by block number. All errors are fatal (including rate-limit exhaustion).
func fetchBlocks(ctx context.Context, c Client, nums []uint64, concurrency int64) (map[uint64]*types.Block, error) {
	sem := semaphore.NewWeighted(concurrency)
	g, gctx := errgroup.WithContext(ctx)
	out := make(map[uint64]*types.Block, len(nums))
	var mu sync.Mutex

	for _, n := range nums {
		n := n
		if err := sem.Acquire(gctx, 1); err != nil {
			return nil, fmt.Errorf("acquire sem: %w", err)
		}
		g.Go(func() error {
			defer sem.Release(1)
			var blk *types.Block
			err := WithRateLimitBackoff(gctx, func() error {
				var berr error
				blk, berr = c.BlockByNumber(gctx, n)
				return berr
			}, DefaultBackoff)
			if err != nil {
				return fmt.Errorf("block %d: %w", n, err)
			}
			mu.Lock()
			out[n] = blk
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// fetchBlockReceipts fetches receipts for the given block numbers concurrently.
func fetchBlockReceipts(ctx context.Context, c Client, nums []uint64, concurrency int64) (map[uint64][]*types.Receipt, error) {
	sem := semaphore.NewWeighted(concurrency)
	g, gctx := errgroup.WithContext(ctx)
	out := make(map[uint64][]*types.Receipt, len(nums))
	var mu sync.Mutex

	for _, n := range nums {
		n := n
		if err := sem.Acquire(gctx, 1); err != nil {
			return nil, fmt.Errorf("acquire sem: %w", err)
		}
		g.Go(func() error {
			defer sem.Release(1)
			var rs []*types.Receipt
			err := WithRateLimitBackoff(gctx, func() error {
				var berr error
				rs, berr = c.BlockReceipts(gctx, n)
				return berr
			}, DefaultBackoff)
			if err != nil {
				return fmt.Errorf("receipts block %d: %w", n, err)
			}
			mu.Lock()
			out[n] = rs
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// txByHash returns the transaction with the given hash from the block, or nil.
func txByHash(blk *types.Block, hash common.Hash) *types.Transaction {
	for _, tx := range blk.Transactions() {
		if tx.Hash() == hash {
			return tx
		}
	}
	return nil
}

// receiptGasUsed returns the GasUsed for the transaction with the given hash,
// or 0 if not found.
func receiptGasUsed(rs []*types.Receipt, hash common.Hash) uint64 {
	for _, r := range rs {
		if r.TxHash == hash {
			return r.GasUsed
		}
	}
	return 0
}

// receiptEffectiveGasPrice returns the EffectiveGasPrice from a receipt, or
// nil if the receipt is nil or has no effective gas price set.
func receiptEffectiveGasPrice(r *types.Receipt) *big.Int {
	if r == nil {
		return nil
	}
	return r.EffectiveGasPrice
}
