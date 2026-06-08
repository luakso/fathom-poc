package base

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/x402"
)

func TestHyperSyncQuery_JSONShape(t *testing.T) {
	t.Parallel()
	q := BuildBackfillQuery(40_222_720, 40_222_820)

	bs, err := json.Marshal(q)
	require.NoError(t, err)

	// Re-decode into a generic map and assert the fields HyperSync expects.
	var got map[string]any
	require.NoError(t, json.Unmarshal(bs, &got))

	require.Equal(t, float64(40_222_720), got["from_block"])
	require.Equal(t, float64(40_222_820), got["to_block"])

	logs := got["logs"].([]any)
	require.Len(t, logs, 1)
	logFilter := logs[0].(map[string]any)
	addrs := logFilter["address"].([]any)
	require.Len(t, addrs, 1)
	require.Equal(t, "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", addrs[0])

	topics := logFilter["topics"].([]any)
	require.Len(t, topics, 1)
	topic0 := topics[0].([]any)
	require.Len(t, topic0, 1)
	require.Equal(t, x402.AuthorizationUsedTopic.Hex(), topic0[0])

	// Selection is log-only — no transaction-level sighash filter. The query
	// must omit "transactions" entirely (companions arrive via the JoinAll log
	// join, not a tx match), so a reader can't mistake a hint for a keep-filter.
	_, hasTxFilter := got["transactions"]
	require.False(t, hasTxFilter, "query must not carry a transactions filter")

	// field_selection.log must use discrete topic0..topic3 columns — HyperSync
	// rejects the "topics" variant with HTTP 400.
	fieldSel := got["field_selection"].(map[string]any)
	logFields := fieldSel["log"].([]any)
	require.Contains(t, logFields, "topic0")
	require.Contains(t, logFields, "topic1")
	require.Contains(t, logFields, "topic2")
	require.Contains(t, logFields, "topic3")
	require.NotContains(t, logFields, "topics")

	// field_selection.transaction must request the EIP-1559 fee caps so we can
	// reconstruct facilitator bid-vs-paid economics, not just what was paid.
	txFields := fieldSel["transaction"].([]any)
	require.Contains(t, txFields, "effective_gas_price")
	require.Contains(t, txFields, "max_fee_per_gas")
	require.Contains(t, txFields, "max_priority_fee_per_gas")

	// join_mode must be JoinAll so HyperSync returns each matched log's parent
	// tx AND that tx's sibling logs — including the companion Transfer pairing
	// needs. The log filter alone returns only AuthorizationUsed logs.
	require.Equal(t, "JoinAll", got["join_mode"])
}

// TestHyperSyncBatch_DecodesRealWireShape locks in the real HyperSync /query
// response shape: `data` is an ARRAY of {logs,transactions,blocks} chunks, and
// indexed log topics arrive as discrete topic0..topic3 fields (no `topics`
// array). The body below is trimmed from a live Base response.
func TestHyperSyncBatch_DecodesRealWireShape(t *testing.T) {
	t.Parallel()
	body := []byte(`{
		"data": [
			{
				"logs": [
					{"log_index":168,"transaction_index":45,"transaction_hash":"0x7348c4695aa695a22298ccb0b382f25e720b1e25a0282a308b502587929c7225","block_number":46618000,"address":"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913","data":"0x","topic0":"0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5","topic1":"0x000000000000000000000000b90a408b5811d302c08c0aa2c6e6c757f87e9ae4","topic2":"0x201c19d42de40583dcae582c5ab2775811399e41508ba7dd1de67ced33e02c7a"}
				],
				"transactions": [
					{"hash":"0x7348c4695aa695a22298ccb0b382f25e720b1e25a0282a308b502587929c7225","block_number":46618000,"from":"0xb90a408b5811d302c08c0aa2c6e6c757f87e9ae4","to":"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913","input":"0xe3ee160edeadbeef","type":2,"nonce":"0xa","gas_used":"0xc350","effective_gas_price":"0x3b9aca00"}
				],
				"blocks": [ {"number":46618000,"timestamp":"0x6553f100","hash":"0xblock0","base_fee_per_gas":"0x1dcd6500"} ]
			},
			{
				"logs": [
					{"log_index":5,"transaction_index":1,"transaction_hash":"0xaaaa","block_number":46618001,"address":"0x833589fcd6edb6e08f4c7c32d4f71b54bda02913","data":"0x","topic0":"0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5","topic1":"0x000000000000000000000000b90a408b5811d302c08c0aa2c6e6c757f87e9ae4","topic2":"0x201c19d42de40583dcae582c5ab2775811399e41508ba7dd1de67ced33e02c7a"}
				],
				"transactions": [],
				"blocks": [ {"number":46618001,"timestamp":"0x6553f102","hash":"0xblock1"} ]
			}
		],
		"archive_height": 46666597,
		"next_block": 46618637,
		"total_execution_time": 67,
		"rollback_guard": {"block_number": 46666597}
	}`)

	var batch HyperSyncBatch
	require.NoError(t, json.Unmarshal(body, &batch))

	// data array flattened into one batch
	require.Len(t, batch.Data.Logs, 2, "logs flattened across data chunks")
	require.Len(t, batch.Data.Transactions, 1)
	require.Len(t, batch.Data.Blocks, 2)
	require.Equal(t, uint64(46618637), batch.NextBlock)
	require.Equal(t, uint64(46666597), batch.ArchiveHeight)
	require.Equal(t, uint64(46618001), batch.MaxBlock())

	// topic0..topic3 assembled into Topics (topic3 absent → 3 entries)
	require.Len(t, batch.Data.Logs[0].Topics, 3)
	require.Equal(t, x402.AuthorizationUsedTopic.Hex(), batch.Data.Logs[0].Topics[0])

	// hex-string quantity fields decode to typed values
	tx, err := ConvertTransaction(batch.Data.Transactions[0])
	require.NoError(t, err)
	require.Equal(t, uint64(10), tx.Nonce)       // 0xa
	require.Equal(t, uint64(50_000), tx.GasUsed) // 0xc350
	blk, err := ConvertBlock(batch.Data.Blocks[0])
	require.NoError(t, err)
	require.Equal(t, uint64(1_700_000_000), blk.Timestamp)       // 0x6553f100
	require.Equal(t, big.NewInt(500_000_000), blk.BaseFeePerGas) // 0x1dcd6500
}

func TestSighashHex(t *testing.T) {
	t.Parallel()
	require.Equal(t, "0xe3ee160e", SighashHex(0xe3ee160e))
	require.Equal(t, "0x00000001", SighashHex(0x00000001))
}

func TestHyperSyncBatch_MaxBlock(t *testing.T) {
	t.Parallel()
	require.Equal(t, uint64(0), HyperSyncBatch{}.MaxBlock(), "empty batch must report 0 — the empty-batch guard depends on this")

	b := HyperSyncBatch{Data: HyperSyncBatchData{Blocks: []HyperSyncBlock{
		{Number: 100}, {Number: 250}, {Number: 175},
	}}}
	require.Equal(t, uint64(250), b.MaxBlock())
}
