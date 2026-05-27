package base

import (
	"encoding/json"
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

	txFilters := got["transactions"].([]any)
	require.Len(t, txFilters, 1)
	txFilter := txFilters[0].(map[string]any)
	sigList := txFilter["sighash"].([]any)
	require.Len(t, sigList, len(x402.AllowSighashes))
	require.Equal(t, SighashHex(x402.AllowSighashes[0]), sigList[0])
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
