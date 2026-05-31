package x402

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

const (
	addrA = "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	addrB = "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	addrC = "0x000000000000000000000000cccc000000000000000000000000000000000001"
	addrD = "0x000000000000000000000000dddd000000000000000000000000000000000001"
	addrE = "0x000000000000000000000000eeee000000000000000000000000000000000001"
	addrF = "0x000000000000000000000000ffff000000000000000000000000000000000001"
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

// makeAuth builds an AuthorizationUsed log fixture. Both event params are
// indexed, so the real log has 3 topics (sig, authorizer, nonce) and no data.
func makeAuth(t *testing.T, logIndex uint32, authorizer string) Log {
	t.Helper()
	nonce := make([]byte, 32)
	nonce[0] = 0xaa
	return Log{
		Address: USDCProxyBase,
		Topics: []common.Hash{
			AuthorizationUsedTopic,
			common.HexToHash(authorizer),
			common.BytesToHash(nonce),
		},
		LogIndex: logIndex,
	}
}

// Real USDC EIP-3009 emission order: _markAuthorizationAsUsed (AuthorizationUsed)
// runs BEFORE _transfer (Transfer), so the companion Transfer always FOLLOWS the
// auth at the next USDC log_index. These tests encode that on-chain order.

func TestPairCompanionTransfer_SinglePayment(t *testing.T) {
	t.Parallel()
	logs := []Log{
		makeAuth(t, 5, addrA),
		makeTransfer(t, 6, addrA, addrB, 100),
	}
	got, ok := PairCompanionTransfer(logs, 5)
	require.True(t, ok)
	require.Equal(t, uint32(6), got.LogIndex, "companion Transfer follows the auth at the next index")
}

func TestPairCompanionTransfer_MulticallInterleaved(t *testing.T) {
	t.Parallel()
	// Two payments interleaved: [AUTH0@0, XFER0@1, AUTH1@2, XFER1@3].
	// auth@0 must pair with transfer@1; auth@2 must pair with the FOLLOWING
	// transfer@3 — NOT the earlier transfer@1 that belongs to the first payment.
	logs := []Log{
		makeAuth(t, 0, addrA),
		makeTransfer(t, 1, addrA, addrB, 100),
		makeAuth(t, 2, addrC),
		makeTransfer(t, 3, addrC, addrD, 500),
	}
	a, ok := PairCompanionTransfer(logs, 0)
	require.True(t, ok)
	require.Equal(t, uint32(1), a.LogIndex, "auth@0 must pair with transfer@1")

	b, ok := PairCompanionTransfer(logs, 2)
	require.True(t, ok)
	require.Equal(t, uint32(3), b.LogIndex, "auth@2 must pair with the FOLLOWING transfer@3, not the earlier @1")
}

func TestPairCompanionTransfer_IgnoresTransferBefore(t *testing.T) {
	t.Parallel()
	// A USDC Transfer with log_index < auth (a preceding payment or fee
	// forwarding) is never the companion — the matching Transfer always follows.
	logs := []Log{
		makeTransfer(t, 0, addrE, addrF, 999), // preceding, unrelated
		makeAuth(t, 1, addrA),
		makeTransfer(t, 2, addrA, addrB, 100), // this auth's transfer
	}
	got, ok := PairCompanionTransfer(logs, 1)
	require.True(t, ok)
	require.Equal(t, uint32(2), got.LogIndex, "must pick the following transfer, not the preceding one")
}

func TestPairCompanionTransfer_IgnoresNonUSDC(t *testing.T) {
	t.Parallel()
	other := common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	logs := []Log{
		makeAuth(t, 0, addrA),
		// Non-USDC Transfer right after the auth — must be ignored.
		{
			Address:  other,
			Topics:   []common.Hash{TransferTopic, common.HexToHash("0x0"), common.HexToHash("0x0")},
			Data:     make([]byte, 32),
			LogIndex: 1,
		},
		makeTransfer(t, 2, addrA, addrB, 100),
	}
	got, ok := PairCompanionTransfer(logs, 0)
	require.True(t, ok)
	require.Equal(t, uint32(2), got.LogIndex, "USDC Transfer at log_index=2 wins; non-USDC at 1 is ignored")
}

func TestPairCompanionTransfer_NoMatch(t *testing.T) {
	t.Parallel()
	// An auth with no FOLLOWING USDC Transfer has no companion. A Transfer that
	// precedes the auth does not count.
	logs := []Log{
		makeTransfer(t, 0, addrA, addrB, 100),
		makeAuth(t, 1, addrA),
	}
	_, ok := PairCompanionTransfer(logs, 1)
	require.False(t, ok)
}
