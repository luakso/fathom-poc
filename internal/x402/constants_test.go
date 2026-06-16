package x402

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// Asserts every sighash constant with a known signature equals
// keccak256(signature)[0..4]. Catches name-vs-bytes drift forever.
//
// SighashUnattributedX (0x93d9c747) is intentionally NOT here — the signature
// is unknown; the constant is empirical.
func TestSelectorsMatchKeccak(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sig  string
		want uint32
	}{
		{
			name: "transferWithAuthorization classic-sig",
			sig:  "transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)",
			want: SighashTransferWithAuthV,
		},
		{
			name: "transferWithAuthorization bytes-sig",
			sig:  "transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,bytes)",
			want: SighashTransferWithAuthB,
		},
		{
			name: "receiveWithAuthorization classic-sig",
			sig:  "receiveWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)",
			want: SighashReceiveWithAuthV,
		},
		{
			name: "receiveWithAuthorization bytes-sig",
			sig:  "receiveWithAuthorization(address,address,uint256,uint256,uint256,bytes32,bytes)",
			want: SighashReceiveWithAuthB,
		},
		{
			name: "Multicall3.aggregate3",
			sig:  "aggregate3((address,bool,bytes)[])",
			want: SighashAggregate3,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := binary.BigEndian.Uint32(crypto.Keccak256([]byte(c.sig))[:4])
			require.Equal(t, c.want, got, c.sig)
		})
	}
}

func TestEventTopicsMatchKeccak(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sig  string
		want common.Hash
	}{
		{
			name: "AuthorizationUsed",
			sig:  "AuthorizationUsed(address,bytes32)",
			want: AuthorizationUsedTopic,
		},
		{
			name: "Transfer",
			sig:  "Transfer(address,address,uint256)",
			want: TransferTopic,
		},
		{
			name: "AuthorizationCanceled",
			sig:  "AuthorizationCanceled(address,bytes32)",
			want: AuthorizationCanceledTopic,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := crypto.Keccak256Hash([]byte(c.sig))
			require.Equal(t, c.want, got, c.sig)
		})
	}
}

func TestAllowDenyDisjoint(t *testing.T) {
	t.Parallel()
	for _, a := range AllowSighashes {
		for _, d := range DenySighashes {
			require.NotEqual(t, a, d, "allow and deny lists must be disjoint: 0x%08x", a)
		}
	}
}
