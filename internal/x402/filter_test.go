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

func TestKeepAuthorizationUsed_DroppedByReceiveSelector(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0xef, 0x55, 0xbe, 0xc6}))
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0x88, 0xb7, 0xab, 0x63}))
}

func TestKeepAuthorizationUsed_DroppedByUnknownSelector(t *testing.T) {
	t.Parallel()
	authLog := Log{
		Address: USDCProxyBase,
		Topics:  []common.Hash{AuthorizationUsedTopic, common.HexToHash("0x01")},
	}
	require.False(t, KeepAuthorizationUsed(authLog, []byte{0xde, 0xad, 0xbe, 0xef}))
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
