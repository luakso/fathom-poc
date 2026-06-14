package x402

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestKeepAuthorizationUsed_HappyPath(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	parentInput := []byte{0xe3, 0xee, 0x16, 0x0e} // classic transferWithAuthorization
	require.True(t, KeepAuthorizationUsed(authLog, parentInput))
}

func TestKeepAuthorizationUsed_BytesOverloadKept(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	parentInput := []byte{0xcf, 0x09, 0x29, 0x95}
	require.True(t, KeepAuthorizationUsed(authLog, parentInput))
}

func TestKeepAuthorizationUsed_Aggregate3Kept(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	parentInput := []byte{0x82, 0xad, 0x56, 0xcb}
	require.True(t, KeepAuthorizationUsed(authLog, parentInput))
}

func TestKeepAuthorizationUsed_UnattributedKept(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	parentInput := []byte{0x93, 0xd9, 0xc7, 0x47}
	require.True(t, KeepAuthorizationUsed(authLog, parentInput))
}

// receiveWithAuthorization is self-settled x402, no longer denied — it is kept
// and flagged settlement_kind='receive' downstream (see SettlementKind).
func TestKeepAuthorizationUsed_KeepsReceiveWithAuthorization(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.True(t, KeepAuthorizationUsed(authLog, []byte{0xef, 0x55, 0xbe, 0xc6}))
	require.True(t, KeepAuthorizationUsed(authLog, []byte{0x88, 0xb7, 0xab, 0x63}))
}

// Topic-only policy (c-simple): a selector we've never catalogued still emits a
// real AuthorizationUsed-on-USDC payment, so it is KEPT. This is the opposite of
// the old allow-list behavior and is what recovers the ~4.5% coverage gap.
func TestKeepAuthorizationUsed_UnknownSelectorKept(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.True(t, KeepAuthorizationUsed(authLog, []byte{0xde, 0xad, 0xbe, 0xef}))
}

// ERC-4337 handleOps wraps a transferWithAuthorization settlement — the dominant
// previously-dropped selector. Topic-only keeps it.
func TestKeepAuthorizationUsed_HandleOpsKept(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.True(t, KeepAuthorizationUsed(authLog, []byte{0x1f, 0xad, 0x94, 0x8c}))
}

// An input too short to carry a 4-byte selector is dropped: it can't be a real
// USDC settlement, and downstream row-building reads parentTxInput[:4], so
// keeping it would feed a malformed row (and previously could panic). Dropping
// here preserves the invariant that a kept event always has a >=4-byte selector.
func TestKeepAuthorizationUsed_ShortInputDropped(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0x01, 0x02}))
}

func TestKeepAuthorizationUsed_DroppedByWrongAddress(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: common.HexToAddress("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0xe3, 0xee, 0x16, 0x0e}))
}

func TestKeepAuthorizationUsed_DroppedByWrongTopic(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{TransferTopic, common.HexToHash("0x01")},
	}
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0xe3, 0xee, 0x16, 0x0e}))
}
