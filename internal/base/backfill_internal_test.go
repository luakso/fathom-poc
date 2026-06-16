package base

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/lukostrobl/fathom/internal/x402"
)

// allCandidatesLost is the guard predicate: it must fire only when the batch
// carried genuine x402 candidates (AuthLogs beyond the expected Denied drops)
// yet produced zero rows — the signature of a pairing/JoinAll regression.
func TestAllCandidatesLost(t *testing.T) {
	tests := []struct {
		name  string
		stats x402.AssembleStats
		want  bool
	}{
		{"no logs at all", x402.AssembleStats{}, false},
		{"normal: all kept", x402.AssembleStats{AuthLogs: 3, Kept: 3}, false},
		{"partial loss still produced rows", x402.AssembleStats{AuthLogs: 3, Kept: 2, Dropped: 1}, false},
		{"denied-only is expected, not loss", x402.AssembleStats{AuthLogs: 2, Denied: 2}, false},
		{"candidates present, zero kept → halt", x402.AssembleStats{AuthLogs: 2, Kept: 0, Dropped: 2}, true},
		{"one denied, one dropped, zero kept → halt", x402.AssembleStats{AuthLogs: 2, Denied: 1, Dropped: 1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, allCandidatesLost(tc.stats))
		})
	}
}

func TestDecodeBatch_RoutesCancellationsSeparately(t *testing.T) {
	usdc := strings.ToLower(x402.USDCProxyBase.Hex())
	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	nonce := "0x1111111111111111111111111111111111111111111111111111111111111111"

	batch := HyperSyncBatch{
		Data: HyperSyncBatchData{
			Logs: []HyperSyncLog{
				{
					Address: usdc, Topics: []string{x402.AuthorizationUsedTopic.Hex(), payer, nonce},
					Data: "0x", BlockNumber: 100, TxHash: "0xdead", TxIndex: 0, LogIndex: 0,
				},
				{
					Address: usdc, Topics: []string{x402.TransferTopic.Hex(), payer, payee},
					Data:        "0x00000000000000000000000000000000000000000000000000000000000f4240",
					BlockNumber: 100, TxHash: "0xdead", TxIndex: 0, LogIndex: 1,
				},
				{
					Address: usdc, Topics: []string{x402.AuthorizationCanceledTopic.Hex(), payer, nonce},
					Data: "0x", BlockNumber: 100, TxHash: "0xcafe", TxIndex: 1, LogIndex: 0,
				},
			},
			Transactions: []HyperSyncTransaction{
				{
					Hash: "0xdead", BlockNumber: 100, From: "0xfac1000000000000000000000000000000000001",
					To: usdc, Input: "0xe3ee160edeadbeef", Type: 2, Nonce: "0x7",
					GasUsed: "0xc350", EffectiveGasPrice: "0x3b9aca00",
				},
				{
					Hash: "0xcafe", BlockNumber: 100, From: "0xfac1000000000000000000000000000000000001",
					To: usdc, Input: "0x", Type: 2, Nonce: "0x8",
					GasUsed: "0x5208", EffectiveGasPrice: "0x3b9aca00",
				},
			},
			Blocks: []HyperSyncBlock{
				{Number: 100, Timestamp: "0x6553f100", Hash: "0xb100", BaseFeePerGas: "0x1dcd6500"},
			},
		},
		NextBlock: 101,
	}

	// TxHash is built as strings.ToLower(common.Hash.Hex()) — the full zero-padded
	// 66-char form, NOT the abbreviated wire literal.
	wantCancelTx := strings.ToLower(common.HexToHash("0xcafe").Hex())

	payments, cancellations, _, err := decodeBatch(batch)
	require.NoError(t, err)
	require.Len(t, payments, 1, "the AuthorizationUsed tx still yields exactly one payment")
	require.Len(t, cancellations, 1, "the AuthorizationCanceled log yields one cancellation")
	require.Equal(t, wantCancelTx, cancellations[0].TxHash)
	for _, p := range payments {
		require.NotEqual(t, wantCancelTx, p.TxHash, "cancel tx must not appear as a payment")
	}
}
