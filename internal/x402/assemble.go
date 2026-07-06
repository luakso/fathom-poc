package x402

import (
	"log/slog"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/shopspring/decimal"
)

// AssembleStats reports what Assemble did with the AuthorizationUsed logs it
// saw, so callers can detect silent data loss. The invariant
//
//	AuthLogs == Denied + Kept + Dropped
//
// always holds. Denied are EXPECTED drops (no selector — input shorter than 4
// bytes, cannot be a real USDC settlement). Dropped are ANOMALOUS drops: a candidate
// that passed the keep filter but produced no row (missing parent tx, no
// companion Transfer, decode failure, payer mismatch, missing block). A batch
// with candidates (AuthLogs > Denied) but Kept == 0 means companion pairing
// has regressed — the backfill guard refuses to advance the cursor on that.
type AssembleStats struct {
	AuthLogs int
	Denied   int
	Kept     int
	Dropped  int
}

// Assemble takes pre-fetched logs/transactions/blocks and returns the
// []Payment rows ready for Store.InsertBatch, plus AssembleStats describing
// how many candidate AuthorizationUsed logs were kept vs dropped.
//
// It applies the §5 filter (KeepAuthorizationUsed), pairs each surviving
// AuthorizationUsed with its companion Transfer (§9 pairing rule), and
// constructs Payment rows with all routing + gas metadata captured at
// observation time.
//
// Per-row failures (decode errors, missing companion, missing tx/block)
// log a warn and skip the row — never abort the batch. This matches the
// spec §11 "decode failure on one row" policy.
//
// Inputs:
//   - allLogs:       every log returned by the fetch layer (any address/topic).
//     Assemble filters internally — callers don't need to pre-filter.
//   - txByHash:      parent transactions keyed by tx hash.
//   - receiptByHash: full ordered log list per receipt, keyed by tx hash.
//     Populated from the HyperSync per-tx log slice.
//   - blockByNumber: block metadata keyed by block number.
//
// Output is ordered by (block_number, log_index).
func Assemble(
	allLogs []Log,
	txByHash map[common.Hash]Transaction,
	receiptByHash map[common.Hash][]Log,
	blockByNumber map[uint64]Block,
) ([]Payment, AssembleStats) {
	out := make([]Payment, 0, len(allLogs)/2) // rough capacity hint
	stats := AssembleStats{}

	// Collect the candidate AuthorizationUsed logs, then process them in ascending
	// (block_number, log_index) order. On EVM log_index is unique and monotonic
	// within a block, so this is a total order that, within each receipt, visits
	// the earliest auth first. That ordering is LOAD-BEARING for the consumed-
	// Transfer accounting below: the earliest auth claims its companion Transfer
	// before any later auth in the same receipt can.
	candidates := make([]Log, 0, len(allLogs)/2)
	for _, lg := range allLogs {
		// Topic + address gate. KeepAuthorizationUsed handles the rest.
		if lg.Address != USDCProxyBase {
			continue
		}
		if len(lg.Topics) == 0 || lg.Topics[0] != AuthorizationUsedTopic {
			continue
		}
		candidates = append(candidates, lg)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].BlockNumber != candidates[j].BlockNumber {
			return candidates[i].BlockNumber < candidates[j].BlockNumber
		}
		return candidates[i].LogIndex < candidates[j].LogIndex
	})

	// consumedByTx tracks, per receipt, the log indices of USDC Transfers already
	// bound to a KEPT payment. Each Transfer may back at most one payment row, so
	// a malformed/adversarial receipt (e.g. [AUTH0, AUTH1, XFER] where AUTH0 and
	// AUTH1 share an authorizer) can never write the same transfer amount to two
	// rows — that would invent money in a public, citable figure. A Transfer is
	// marked consumed only when its auth is actually kept, so an auth that later
	// drops does not steal a Transfer from the payment it truly belongs to.
	consumedByTx := map[common.Hash]map[uint32]bool{}

	for _, lg := range candidates {
		stats.AuthLogs++

		tx, ok := txByHash[lg.TxHash]
		if !ok {
			slog.Warn("assemble: missing parent tx", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			stats.Dropped++
			continue
		}
		if !KeepAuthorizationUsed(lg, tx.Input) {
			stats.Denied++
			continue
		}

		receiptLogs, ok := receiptByHash[lg.TxHash]
		if !ok {
			slog.Warn("assemble: missing receipt logs", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			stats.Dropped++
			continue
		}

		consumed := consumedByTx[lg.TxHash]
		companion, ok := PairCompanionTransfer(receiptLogs, lg.LogIndex, consumed)
		if !ok {
			// Either no following USDC Transfer exists, or the only candidate was
			// already claimed by an earlier auth in this receipt. Reaching forward
			// to a Transfer that belongs to another payment would double-count it,
			// so drop this auth as anomalous instead.
			slog.Warn("assemble: no available companion Transfer (missing, or already consumed by an earlier authorization)",
				"tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			stats.Dropped++
			continue
		}

		from, to, value, err := DecodeTransfer(companion)
		if err != nil {
			slog.Warn("assemble: decode Transfer failed", "tx_hash", lg.TxHash.Hex(), "err", err)
			stats.Dropped++
			continue
		}
		authorizer, nonce, err := DecodeAuthorizationUsed(lg)
		if err != nil {
			slog.Warn("assemble: decode AuthorizationUsed failed", "tx_hash", lg.TxHash.Hex(), "err", err)
			stats.Dropped++
			continue
		}
		if authorizer != from {
			slog.Warn("assemble: authorizer != Transfer.from — skipping",
				"tx_hash", lg.TxHash.Hex(), "authorizer", authorizer.Hex(), "transfer_from", from.Hex())
			stats.Dropped++
			continue
		}

		block, ok := blockByNumber[lg.BlockNumber]
		if !ok {
			slog.Warn("assemble: missing block context", "tx_hash", lg.TxHash.Hex(), "block_number", lg.BlockNumber)
			stats.Dropped++
			continue
		}
		if block.Timestamp > math.MaxInt64 {
			slog.Warn("assemble: block timestamp overflows int64", "tx_hash", lg.TxHash.Hex(), "timestamp", block.Timestamp)
			stats.Dropped++
			continue
		}

		gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.GasUsed), tx.EffectiveGasPrice)

		methodSel := make([]byte, 4)
		copy(methodSel, tx.Input[:4])

		settlementKind := ClassifySettlement(tx.Input)
		selfSettled := tx.From == to
		validAfterRaw, validBeforeRaw, _ := DecodeAuthorizationWindow(tx.Input)
		inputCopy := append([]byte(nil), tx.Input...)

		out = append(out, Payment{
			Chain:          ChainBase,
			TxHash:         strings.ToLower(lg.TxHash.Hex()),
			LogIndex:       lg.LogIndex,
			BlockNumber:    lg.BlockNumber,
			BlockTimestamp: time.Unix(int64(block.Timestamp), 0).UTC(), //nolint:gosec // bounds-checked above
			Source:         SourceBaseCollector,
			Protocol:       ProtocolX402,
			Facilitator:    strings.ToLower(tx.From.Hex()),
			Payer:          strings.ToLower(from.Hex()),
			Payee:          strings.ToLower(to.Hex()),
			PayeeServiceID: nil,
			Asset:          AssetUSDC,
			TokenAddress:   strings.ToLower(USDCProxyBase.Hex()),
			AmountRaw:      value,
			AmountUSDC:     USDCFromRaw(value),
			AssetUSDAtTime: decimal.NewFromInt(1),
			AuthNonce:      nonce,
			MethodSelector: methodSel,
			CalledContract: strings.ToLower(tx.To.Hex()),
			TxType:         tx.Type,
			TxNonce:        tx.Nonce,
			GasUsed:        tx.GasUsed,
			// Money/gas big.Ints are copied out of the per-batch tx/block maps so
			// the Payment owns its values — no aliasing into shared, mutable batch
			// state (matches the defensive copies of InputCalldata/AuthNonce).
			EffectiveGasPrice:    cloneBig(tx.EffectiveGasPrice),
			GasCostWei:           gasCost, // freshly allocated above
			BaseFeePerGas:        cloneBig(block.BaseFeePerGas),
			MaxFeePerGas:         cloneBig(tx.MaxFeePerGas),
			MaxPriorityFeePerGas: cloneBig(tx.MaxPriorityFeePerGas),
			SettlementKind:       settlementKind,
			SelfSettled:          selfSettled,
			ValidAfter:           unixToTimePtr(validAfterRaw),
			ValidBefore:          unixToTimePtr(validBeforeRaw),
			InputCalldata:        inputCopy,
			BlockHash:            strings.ToLower(block.Hash.Hex()),
			TransactionIndex:     lg.TxIndex,
			TokenDecimals:        USDCDecimals,
			TokenSymbol:          TokenSymbolUSDC,
			TxValue:              cloneBig(tx.Value),
			GasLimit:             tx.GasLimit,
			L1Fee:                cloneBig(tx.L1Fee),
			L1GasUsed:            cloneBig(tx.L1GasUsed),
			L1GasPrice:           cloneBig(tx.L1GasPrice),
		})

		// The companion Transfer is now spoken for: mark it consumed so no later
		// auth in this receipt can pair to it.
		if consumed == nil {
			consumed = map[uint32]bool{}
			consumedByTx[lg.TxHash] = consumed
		}
		consumed[companion.LogIndex] = true
		stats.Kept++
	}

	// Guarantee the documented output ordering. Kept rows are appended in the
	// candidate order (already ascending by block/log_index), so this is defensive
	// — it keeps the (block_number, log_index) contract true regardless of how the
	// loop above evolves.
	sort.Slice(out, func(i, j int) bool {
		if out[i].BlockNumber != out[j].BlockNumber {
			return out[i].BlockNumber < out[j].BlockNumber
		}
		return out[i].LogIndex < out[j].LogIndex
	})
	return out, stats
}

// cloneBig returns an independent copy of v (nil-safe), so an assembled Payment
// never aliases a *big.Int owned by the per-batch tx/block maps.
func cloneBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}
