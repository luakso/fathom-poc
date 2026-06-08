package base

import (
	"encoding/json"
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
	// JoinMode controls which related rows HyperSync returns. "JoinAll" makes it
	// return ALL logs of the matched transactions (not just the topic-matched
	// AuthorizationUsed logs), so the companion USDC Transfer needed for pairing
	// is present. Without it the response carries only AuthorizationUsed logs and
	// every row drops at the pairing step.
	JoinMode string `json:"join_mode,omitempty"`
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
	Data          HyperSyncBatchData
	ArchiveHeight uint64
	NextBlock     uint64
}

// UnmarshalJSON flattens HyperSync's wire response into a single batch.
// HyperSync returns `data` as an ARRAY of {logs,transactions,blocks} chunks
// (one query response may carry several), not a single object — so we
// concatenate them and present one flat HyperSyncBatchData to downstream
// consumers (backfill.go, probe.go, MaxBlock).
func (b *HyperSyncBatch) UnmarshalJSON(raw []byte) error {
	var wire struct {
		Data          []HyperSyncBatchData `json:"data"`
		ArchiveHeight uint64               `json:"archive_height"`
		NextBlock     uint64               `json:"next_block"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return err
	}
	var flat HyperSyncBatchData
	for _, d := range wire.Data {
		flat.Logs = append(flat.Logs, d.Logs...)
		flat.Transactions = append(flat.Transactions, d.Transactions...)
		flat.Blocks = append(flat.Blocks, d.Blocks...)
	}
	*b = HyperSyncBatch{
		Data:          flat,
		ArchiveHeight: wire.ArchiveHeight,
		NextBlock:     wire.NextBlock,
	}
	return nil
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

// HyperSyncLog holds raw wire-format log fields. Numeric fields under 64-bit
// width (TxIndex, LogIndex) and byte/256-bit fields (Address, Topics, Data,
// TxHash) arrive as JSON numbers and 0x-prefixed hex strings respectively;
// conversion to x402 types happens in hypersync_decode.go.
//
// Topics is the in-memory representation; on the wire HyperSync ships indexed
// topics as separate topic0..topic3 fields (NOT a `topics` array) — see
// UnmarshalJSON.
type HyperSyncLog struct {
	Address     string
	Topics      []string
	Data        string
	BlockNumber uint64
	TxHash      string
	TxIndex     uint32
	LogIndex    uint32
}

// UnmarshalJSON maps HyperSync's wire log shape into a HyperSyncLog. Indexed
// topics arrive as discrete topic0..topic3 fields; EVM topics are contiguous
// from topic0 (a log can't have topic2 without topic1), so we append the
// non-empty leading slots in order. Requesting "topics" in field_selection is
// rejected by HyperSync with HTTP 400 — see BuildBackfillQuery.
func (l *HyperSyncLog) UnmarshalJSON(b []byte) error {
	var raw struct {
		Address     string `json:"address"`
		Topic0      string `json:"topic0"`
		Topic1      string `json:"topic1"`
		Topic2      string `json:"topic2"`
		Topic3      string `json:"topic3"`
		Data        string `json:"data"`
		BlockNumber uint64 `json:"block_number"`
		TxHash      string `json:"transaction_hash"`
		TxIndex     uint32 `json:"transaction_index"`
		LogIndex    uint32 `json:"log_index"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	topics := make([]string, 0, 4)
	for _, t := range []string{raw.Topic0, raw.Topic1, raw.Topic2, raw.Topic3} {
		if t == "" {
			break
		}
		topics = append(topics, t)
	}
	*l = HyperSyncLog{
		Address:     raw.Address,
		Topics:      topics,
		Data:        raw.Data,
		BlockNumber: raw.BlockNumber,
		TxHash:      raw.TxHash,
		TxIndex:     raw.TxIndex,
		LogIndex:    raw.LogIndex,
	}
	return nil
}

// HyperSyncTransaction holds wire-format transaction fields. HyperSync returns
// EVM quantity fields (nonce, gas_used, effective_gas_price) as 0x-prefixed hex
// STRINGS, while block_number and type arrive as JSON numbers; conversion to
// typed values happens in ConvertTransaction.
type HyperSyncTransaction struct {
	Hash                 string `json:"hash"`
	BlockNumber          uint64 `json:"block_number"`
	From                 string `json:"from"`
	To                   string `json:"to"`
	Input                string `json:"input"`
	Type                 uint8  `json:"type"`
	Nonce                string `json:"nonce"`
	GasUsed              string `json:"gas_used"`
	EffectiveGasPrice    string `json:"effective_gas_price"`
	MaxFeePerGas         string `json:"max_fee_per_gas"`          // empty on legacy/EIP-2930 txs
	MaxPriorityFeePerGas string `json:"max_priority_fee_per_gas"` // empty on legacy/EIP-2930 txs
}

// HyperSyncBlock holds wire-format block fields. timestamp and base_fee_per_gas
// arrive as 0x-prefixed hex STRINGS (number is a JSON number); conversion
// happens in ConvertBlock. base_fee_per_gas is a block-level field in
// HyperSync's schema (it is NOT valid under the transaction field selection).
type HyperSyncBlock struct {
	Number        uint64 `json:"number"`
	Timestamp     string `json:"timestamp"`
	Hash          string `json:"hash"`
	BaseFeePerGas string `json:"base_fee_per_gas"`
}

// BuildBackfillQuery constructs the HyperSync query base-collector uses.
//
// The client keep-policy is topic-only (see x402.KeepAuthorizationUsed): every
// AuthorizationUsed-on-USDC log is kept except a direct receiveWithAuthorization.
// The transaction-level sighash hint below does NOT gate the response —
// JoinMode "JoinAll" returns each matched log's parent tx regardless of its
// selector (the probe observed handleOps/charge/etc. parent txs come through),
// so the hint is effectively vestigial. It is left in place because it is proven
// not to drop data; a follow-up can remove it once join semantics are
// reconfirmed against the live endpoint.
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
		JoinMode:  "JoinAll",
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
			Log:         []string{"address", "topic0", "topic1", "topic2", "topic3", "data", "block_number", "transaction_hash", "transaction_index", "log_index"},
			Transaction: []string{"hash", "block_number", "from", "to", "input", "type", "nonce", "gas_used", "effective_gas_price", "max_fee_per_gas", "max_priority_fee_per_gas"},
			Block:       []string{"number", "timestamp", "hash", "base_fee_per_gas"},
		},
	}
}

// SighashHex formats a 4-byte selector as a lowercase 0x-prefixed hex string.
func SighashHex(s uint32) string {
	return fmt.Sprintf("0x%08x", s)
}
