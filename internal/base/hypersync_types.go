package base

import (
	"fmt"
	"strings"

	"github.com/lukostrobl/fathom/internal/x402"
)

// Fetcher streams (logs, transactions, blocks) bundles from HyperSync.
// One Query produces zero-or-more Batches; Next returns (batch, true) per
// chunk and (zero, false) when the stream is exhausted. An error from Next
// is terminal — callers must not retry without re-issuing the Query.
type Fetcher interface {
	Stream(query HyperSyncQuery) (Stream, error)
}

// Stream is the per-query streaming handle. Callers drive it with Next.
type Stream interface {
	// Next blocks until the next batch is available or the stream ends.
	Next() (HyperSyncBatch, bool, error)
	// Close releases any underlying connection / state. Safe to call multiple
	// times; calls after the first are no-ops.
	Close() error
}

// HyperSyncQuery is the wire shape posted to {base_url}/query.
//
// Fields below cover what base-collector needs:
//   - from_block / to_block:   inclusive block range
//   - logs:                    address + topic filter (final gate on USDC AuthorizationUsed)
//   - transactions:            outer-tx sighash allow-list (server-side multi-value filter)
//   - field_selection:         which columns to ship back; smaller = cheaper
//
// HyperSync's actual query schema is broader; this struct intentionally
// includes only the fields we use.
type HyperSyncQuery struct {
	FromBlock      uint64                  `json:"from_block"`
	ToBlock        uint64                  `json:"to_block,omitempty"`
	Logs           []LogFilter             `json:"logs"`
	Transactions   []TransactionFilter     `json:"transactions"`
	FieldSelection HyperSyncFieldSelection `json:"field_selection"`
}

// LogFilter specifies log event criteria (address + topics).
type LogFilter struct {
	Address []string   `json:"address"`
	Topics  [][]string `json:"topics"`
}

// TransactionFilter specifies outer-tx selection criteria (by sighash).
type TransactionFilter struct {
	Sighash []string `json:"sighash"`
}

// HyperSyncFieldSelection narrows the columns returned. Per envio docs each
// section enumerates raw column names; an empty list means "return all".
type HyperSyncFieldSelection struct {
	Log         []string `json:"log,omitempty"`
	Transaction []string `json:"transaction,omitempty"`
	Block       []string `json:"block,omitempty"`
}

// HyperSyncBatch is one response chunk from the stream. The fields are raw
// wire shape — see hypersync_decode.go for the conversion into x402.Log /
// x402.Transaction / x402.Block.
type HyperSyncBatch struct {
	Data          HyperSyncBatchData `json:"data"`
	ArchiveHeight uint64             `json:"archive_height,omitempty"`
	NextBlock     uint64             `json:"next_block,omitempty"`
}

// MaxBlock returns the highest block contained in the batch (0 if empty).
// This is what the spec §7 "empty-batch guard" checks before advancing the
// cursor — writing 0 over a non-zero cursor would reset progress to genesis.
func (b HyperSyncBatch) MaxBlock() uint64 {
	var hi uint64
	for _, blk := range b.Data.Blocks {
		if blk.Number > hi {
			hi = blk.Number
		}
	}
	return hi
}

// HyperSyncBatchData holds the lists of logs, transactions, and blocks in a batch.
type HyperSyncBatchData struct {
	Logs         []HyperSyncLog         `json:"logs"`
	Transactions []HyperSyncTransaction `json:"transactions"`
	Blocks       []HyperSyncBlock       `json:"blocks"`
}

// HyperSyncLog / HyperSyncTransaction / HyperSyncBlock use stringly-typed
// fields because HyperSync wire format encodes numerics as 0x-prefixed hex
// (variable-length) and bytes as 0x-prefixed hex. We accept the strings here
// and convert in hypersync_decode.go (Task 3).
type HyperSyncLog struct {
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
	BlockNumber uint64   `json:"block_number"`
	TxHash      string   `json:"transaction_hash"`
	TxIndex     uint32   `json:"transaction_index"`
	LogIndex    uint32   `json:"log_index"`
}

// HyperSyncTransaction holds wire-format transaction fields with stringly-typed numerics.
type HyperSyncTransaction struct {
	Hash              string `json:"hash"`
	BlockNumber       uint64 `json:"block_number"`
	From              string `json:"from"`
	To                string `json:"to"`
	Input             string `json:"input"`
	Type              uint8  `json:"type"`
	Nonce             uint64 `json:"nonce"`
	GasUsed           uint64 `json:"gas_used"`
	EffectiveGasPrice string `json:"effective_gas_price"`
	BaseFeePerGas     string `json:"base_fee_per_gas"`
}

// HyperSyncBlock holds wire-format block fields.
type HyperSyncBlock struct {
	Number    uint64 `json:"number"`
	Timestamp uint64 `json:"timestamp"`
	Hash      string `json:"hash"`
}

// BuildBackfillQuery constructs the HyperSync query base-collector uses.
// The transaction-level sighash filter is multi-value: allow-list ∪ deny-list
// is applied client-side too (see x402.KeepAuthorizationUsed); HyperSync's
// server-side filter narrows the response so we don't pay bandwidth on
// every AuthorizationUsed-emitting tx whose sighash is unrelated.
//
// fromBlock is inclusive; toBlock is inclusive (HyperSync convention).
func BuildBackfillQuery(fromBlock, toBlock uint64) HyperSyncQuery {
	sighashes := make([]string, 0, len(x402.AllowSighashes))
	for _, s := range x402.AllowSighashes {
		sighashes = append(sighashes, SighashHex(s))
	}
	return HyperSyncQuery{
		FromBlock: fromBlock,
		ToBlock:   toBlock,
		Logs: []LogFilter{
			{
				Address: []string{strings.ToLower(x402.USDCProxyBase.Hex())},
				Topics: [][]string{
					{x402.AuthorizationUsedTopic.Hex()},
				},
			},
		},
		Transactions: []TransactionFilter{
			{Sighash: sighashes},
		},
		FieldSelection: HyperSyncFieldSelection{
			Log:         []string{"address", "topics", "data", "block_number", "transaction_hash", "transaction_index", "log_index"},
			Transaction: []string{"hash", "block_number", "from", "to", "input", "type", "nonce", "gas_used", "effective_gas_price", "base_fee_per_gas"},
			Block:       []string{"number", "timestamp", "hash"},
		},
	}
}

// SighashHex formats a 4-byte selector as a lowercase 0x-prefixed hex string.
func SighashHex(s uint32) string {
	return fmt.Sprintf("0x%08x", s)
}
