package x402

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// makeTransfer builds a Transfer log fixture at a given log_index.
func makeTransfer(t *testing.T, logIndex uint32, from, to string, value uint64) Log {
	t.Helper()
	bs := make([]byte, 32)
	for i := 0; i < 8; i++ {
		bs[31-i] = byte(value >> (8 * i))
	}
	return Log{
		Address: USDCProxyBase,
		Topics: []common.Hash{
			TransferTopic,
			common.HexToHash(from),
			common.HexToHash(to),
		},
		Data:     bs,
		LogIndex: logIndex,
	}
}

// makeAuth builds an AuthorizationUsed log fixture.
func makeAuth(t *testing.T, logIndex uint32, authorizer string) Log {
	t.Helper()
	nonce := make([]byte, 32)
	nonce[0] = 0xaa
	return Log{
		Address: USDCProxyBase,
		Topics: []common.Hash{
			AuthorizationUsedTopic,
			common.HexToHash(authorizer),
		},
		Data:     nonce,
		LogIndex: logIndex,
	}
}

func TestPairCompanionTransfer_SinglePayment(t *testing.T) {
	t.Parallel()
	logs := []Log{
		makeTransfer(t, 0, "0x000000000000000000000000aaaa000000000000000000000000000000000001", "0x000000000000000000000000bbbb000000000000000000000000000000000001", 100),
		makeAuth(t, 1, "0x000000000000000000000000aaaa000000000000000000000000000000000001"),
	}
	got, ok := PairCompanionTransfer(logs, 1)
	require.True(t, ok)
	require.Equal(t, uint32(0), got.LogIndex)
}

func TestPairCompanionTransfer_MulticallInterleaved(t *testing.T) {
	t.Parallel()
	// Two payments interleaved (the multicall case from findings §4).
	// Auth at log_index=1 must pair with Transfer at 0.
	// Auth at log_index=3 must pair with Transfer at 2 (NOT distance-nearest).
	logs := []Log{
		makeTransfer(t, 0, "0x000000000000000000000000aaaa000000000000000000000000000000000001", "0x000000000000000000000000bbbb000000000000000000000000000000000001", 100),
		makeAuth(t, 1, "0x000000000000000000000000aaaa000000000000000000000000000000000001"),
		makeTransfer(t, 2, "0x000000000000000000000000cccc000000000000000000000000000000000001", "0x000000000000000000000000dddd000000000000000000000000000000000001", 500),
		makeAuth(t, 3, "0x000000000000000000000000cccc000000000000000000000000000000000001"),
	}
	a, ok := PairCompanionTransfer(logs, 1)
	require.True(t, ok)
	require.Equal(t, uint32(0), a.LogIndex, "auth@1 must pair with transfer@0")

	b, ok := PairCompanionTransfer(logs, 3)
	require.True(t, ok)
	require.Equal(t, uint32(2), b.LogIndex, "auth@3 must pair with transfer@2, NOT distance-nearest transfer@0")
}

func TestPairCompanionTransfer_IgnoresTransferAfter(t *testing.T) {
	t.Parallel()
	// A Transfer with log_index > auth_log_index is never the match — it
	// belongs to a later payment or to unrelated activity (e.g. settlement-
	// router fee forwarding). Don't fall through.
	logs := []Log{
		makeTransfer(t, 0, "0x000000000000000000000000aaaa000000000000000000000000000000000001", "0x000000000000000000000000bbbb000000000000000000000000000000000001", 100),
		makeAuth(t, 1, "0x000000000000000000000000aaaa000000000000000000000000000000000001"),
		makeTransfer(t, 2, "0x000000000000000000000000eeee000000000000000000000000000000000001", "0x000000000000000000000000ffff000000000000000000000000000000000001", 999), // unrelated, after
	}
	got, ok := PairCompanionTransfer(logs, 1)
	require.True(t, ok)
	require.Equal(t, uint32(0), got.LogIndex, "must pick the preceding transfer, not fall through to the trailing one")
}

func TestPairCompanionTransfer_IgnoresNonUSDC(t *testing.T) {
	t.Parallel()
	other := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	logs := []Log{
		// Non-USDC Transfer right before the auth — must be ignored.
		{
			Address:  other,
			Topics:   []common.Hash{TransferTopic, common.HexToHash("0x0"), common.HexToHash("0x0")},
			Data:     make([]byte, 32),
			LogIndex: 0,
		},
		makeTransfer(t, 1, "0x000000000000000000000000aaaa000000000000000000000000000000000001", "0x000000000000000000000000bbbb000000000000000000000000000000000001", 100),
		makeAuth(t, 2, "0x000000000000000000000000aaaa000000000000000000000000000000000001"),
	}
	got, ok := PairCompanionTransfer(logs, 2)
	require.True(t, ok)
	require.Equal(t, uint32(1), got.LogIndex, "USDC Transfer at log_index=1 wins; non-USDC at 0 is ignored")
}

func TestPairCompanionTransfer_NoMatch(t *testing.T) {
	t.Parallel()
	logs := []Log{
		makeAuth(t, 0, "0x000000000000000000000000aaaa000000000000000000000000000000000001"),
	}
	_, ok := PairCompanionTransfer(logs, 0)
	require.False(t, ok)
}
