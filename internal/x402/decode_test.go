package x402

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// fixtureLog builds a Log struct from human-readable hex strings.
// topics: list of 32-byte topic hashes. data: raw bytes.
func fixtureLog(t *testing.T, address common.Address, topics []string, data string) Log {
	t.Helper()
	t.Parallel()
	hs := make([]common.Hash, 0, len(topics))
	for _, s := range topics {
		hs = append(hs, common.HexToHash(s))
	}
	d, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	require.NoError(t, err)
	return Log{Address: address, Topics: hs, Data: d}
}

func TestDecodeTransfer_Success(t *testing.T) {
	log := fixtureLog(
		t, USDCProxyBase,
		[]string{
			TransferTopic.Hex(),
			"0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"0x000000000000000000000000bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		// 0x0f4240 = 1_000_000 (1 USDC)
		"0x00000000000000000000000000000000000000000000000000000000000f4240",
	)
	from, to, val, err := DecodeTransfer(log)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), from)
	require.Equal(t, common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), to)
	require.Equal(t, big.NewInt(1_000_000), val)
}

func TestDecodeTransfer_RejectsWrongTopicCount(t *testing.T) {
	log := fixtureLog(
		t, USDCProxyBase,
		[]string{TransferTopic.Hex()}, // missing from/to topics
		"0x00",
	)
	_, _, _, err := DecodeTransfer(log)
	require.Error(t, err)
}

func TestDecodeAuthorizationUsed_Success(t *testing.T) {
	log := fixtureLog(
		t, USDCProxyBase,
		[]string{
			AuthorizationUsedTopic.Hex(),
			"0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		// nonce is non-indexed bytes32 in data
		"0x1111111111111111111111111111111111111111111111111111111111111111",
	)
	authorizer, nonce, err := DecodeAuthorizationUsed(log)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), authorizer)
	require.Len(t, nonce, 32)
	require.Equal(t, byte(0x11), nonce[0])
}

func TestDecodeAuthorizationUsed_RejectsShortData(t *testing.T) {
	log := fixtureLog(
		t, USDCProxyBase,
		[]string{
			AuthorizationUsedTopic.Hex(),
			"0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		"0x1111", // too short
	)
	_, _, err := DecodeAuthorizationUsed(log)
	require.Error(t, err)
}

func TestSighashFromBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []byte
		want uint32
		ok   bool
	}{
		{"classic transferWithAuth", []byte{0xe3, 0xee, 0x16, 0x0e, 0xff, 0x00}, 0xe3ee160e, true},
		{"too short", []byte{0xe3, 0xee}, 0, false},
		{"empty", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, ok := SighashFromBytes(c.in)
			require.Equal(t, c.ok, ok)
			if ok {
				require.Equal(t, c.want, got)
			}
		})
	}
}
