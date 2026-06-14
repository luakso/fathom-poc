package x402

import (
	"log/slog"
	"math"
	"math/big"
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

	for _, lg := range allLogs {
		// Topic + address gate. KeepAuthorizationUsed handles the rest.
		if lg.Address != USDCProxyBase {
			continue
		}
		if len(lg.Topics) == 0 || lg.Topics[0] != AuthorizationUsedTopic {
			continue
		}
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

		companion, ok := PairCompanionTransfer(receiptLogs, lg.LogIndex)
		if !ok {
			slog.Warn("assemble: no companion Transfer", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
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

		settlementKind := SettlementKind(tx.Input)
		selfSettled := tx.From == to
		validAfterRaw, validBeforeRaw, _ := DecodeAuthorizationWindow(tx.Input)
		inputCopy := append([]byte(nil), tx.Input...)

		out = append(out, Payment{
			Chain:                ChainBase,
			TxHash:               strings.ToLower(lg.TxHash.Hex()),
			LogIndex:             lg.LogIndex,
			BlockNumber:          lg.BlockNumber,
			BlockTimestamp:       time.Unix(int64(block.Timestamp), 0).UTC(), //nolint:gosec // bounds-checked above
			Source:               "base-collector",
			Protocol:             "x402",
			Facilitator:          strings.ToLower(tx.From.Hex()),
			Payer:                strings.ToLower(from.Hex()),
			Payee:                strings.ToLower(to.Hex()),
			PayeeServiceID:       nil,
			Asset:                "USDC",
			TokenAddress:         strings.ToLower(USDCProxyBase.Hex()),
			AmountRaw:            value,
			AmountUSDC:           USDCFromRaw(value),
			AssetUSDAtTime:       decimal.NewFromInt(1),
			AuthNonce:            nonce,
			MethodSelector:       methodSel,
			CalledContract:       strings.ToLower(tx.To.Hex()),
			TxType:               tx.Type,
			TxNonce:              tx.Nonce,
			GasUsed:              tx.GasUsed,
			EffectiveGasPrice:    tx.EffectiveGasPrice,
			GasCostWei:           gasCost,
			BaseFeePerGas:        block.BaseFeePerGas,
			MaxFeePerGas:         tx.MaxFeePerGas,
			MaxPriorityFeePerGas: tx.MaxPriorityFeePerGas,
			SettlementKind:       settlementKind,
			SelfSettled:          selfSettled,
			ValidAfter:           unixToTimePtr(validAfterRaw),
			ValidBefore:          unixToTimePtr(validBeforeRaw),
			InputCalldata:        inputCopy,
			BlockHash:            strings.ToLower(block.Hash.Hex()),
			TransactionIndex:     lg.TxIndex,
			TokenDecimals:        USDCDecimals,
			TokenSymbol:          "USDC",
		})
		stats.Kept++
	}
	return out, stats
}
