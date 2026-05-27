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

// Assemble takes pre-fetched logs/transactions/blocks and returns the
// []Payment rows ready for Store.InsertBatch.
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
//     Plan 2 (HyperSync) populates this from the per-tx log slice; Plan 3
//     (RPC) populates it from eth_getBlockReceipts.
//   - blockByNumber: block metadata keyed by block number.
//
// Output is ordered by (block_number, log_index).
func Assemble(
	allLogs []Log,
	txByHash map[common.Hash]Transaction,
	receiptByHash map[common.Hash][]Log,
	blockByNumber map[uint64]Block,
) []Payment {
	out := make([]Payment, 0, len(allLogs)/2) // rough capacity hint

	for _, lg := range allLogs {
		// Topic + address gate. KeepAuthorizationUsed handles the rest.
		if lg.Address != USDCProxyBase {
			continue
		}
		if len(lg.Topics) == 0 || lg.Topics[0] != AuthorizationUsedTopic {
			continue
		}

		tx, ok := txByHash[lg.TxHash]
		if !ok {
			slog.Warn("assemble: missing parent tx", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			continue
		}
		if !KeepAuthorizationUsed(lg, tx.Input) {
			continue
		}

		receiptLogs, ok := receiptByHash[lg.TxHash]
		if !ok {
			slog.Warn("assemble: missing receipt logs", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			continue
		}

		companion, ok := PairCompanionTransfer(receiptLogs, lg.LogIndex)
		if !ok {
			slog.Warn("assemble: no companion Transfer", "tx_hash", lg.TxHash.Hex(), "log_index", lg.LogIndex)
			continue
		}

		from, to, value, err := DecodeTransfer(companion)
		if err != nil {
			slog.Warn("assemble: decode Transfer failed", "tx_hash", lg.TxHash.Hex(), "err", err)
			continue
		}
		authorizer, nonce, err := DecodeAuthorizationUsed(lg)
		if err != nil {
			slog.Warn("assemble: decode AuthorizationUsed failed", "tx_hash", lg.TxHash.Hex(), "err", err)
			continue
		}
		if authorizer != from {
			slog.Warn("assemble: authorizer != Transfer.from — skipping",
				"tx_hash", lg.TxHash.Hex(), "authorizer", authorizer.Hex(), "transfer_from", from.Hex())
			continue
		}

		block, ok := blockByNumber[lg.BlockNumber]
		if !ok {
			slog.Warn("assemble: missing block context", "tx_hash", lg.TxHash.Hex(), "block_number", lg.BlockNumber)
			continue
		}
		if block.Timestamp > math.MaxInt64 {
			slog.Warn("assemble: block timestamp overflows int64", "tx_hash", lg.TxHash.Hex(), "timestamp", block.Timestamp)
			continue
		}

		gasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.GasUsed), tx.EffectiveGasPrice)

		methodSel := make([]byte, 4)
		copy(methodSel, tx.Input[:4])

		out = append(out, Payment{
			Chain:             ChainBase,
			TxHash:            strings.ToLower(lg.TxHash.Hex()),
			LogIndex:          lg.LogIndex,
			BlockNumber:       lg.BlockNumber,
			BlockTimestamp:    time.Unix(int64(block.Timestamp), 0).UTC(), //nolint:gosec // bounds-checked above
			Source:            "base-collector",
			Protocol:          "x402",
			Facilitator:       strings.ToLower(tx.From.Hex()),
			Payer:             strings.ToLower(from.Hex()),
			Payee:             strings.ToLower(to.Hex()),
			PayeeServiceID:    nil,
			Asset:             "USDC",
			TokenAddress:      strings.ToLower(USDCProxyBase.Hex()),
			AmountRaw:         value,
			AmountUSDC:        USDCFromRaw(value),
			AssetUSDAtTime:    decimal.NewFromInt(1),
			AuthNonce:         nonce,
			MethodSelector:    methodSel,
			CalledContract:    strings.ToLower(tx.To.Hex()),
			TxType:            tx.Type,
			TxNonce:           tx.Nonce,
			GasUsed:           tx.GasUsed,
			EffectiveGasPrice: tx.EffectiveGasPrice,
			GasCostWei:        gasCost,
			BaseFeePerGas:     tx.BaseFeePerGas,
		})
	}
	return out
}
