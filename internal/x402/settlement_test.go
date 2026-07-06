package x402

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettlementKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input []byte
		want  SettlementKind
	}{
		{"transferWithAuth classic", []byte{0xe3, 0xee, 0x16, 0x0e}, SettlementTransfer},
		{"transferWithAuth bytes", []byte{0xcf, 0x09, 0x29, 0x95}, SettlementTransfer},
		{"receiveWithAuth classic", []byte{0xef, 0x55, 0xbe, 0xc6}, SettlementReceive},
		{"receiveWithAuth bytes", []byte{0x88, 0xb7, 0xab, 0x63}, SettlementReceive},
		{"aggregate3 wrapper", []byte{0x82, 0xad, 0x56, 0xcb}, SettlementTransfer},
		{"no selector", []byte{0x00}, SettlementTransfer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.want, ClassifySettlement(c.input))
		})
	}
}

func TestDecodeAuthorizationWindow(t *testing.T) {
	t.Parallel()

	// Build calldata: selector(4) + from + to + value + validAfter + validBefore + nonce.
	build := func(selector []byte, validAfter, validBefore uint64) []byte {
		out := append([]byte{}, selector...)
		out = append(out, make([]byte, 32)...)              // from
		out = append(out, make([]byte, 32)...)              // to
		out = append(out, make([]byte, 32)...)              // value
		out = append(out, make32WithUint64(validAfter)...)  // validAfter
		out = append(out, make32WithUint64(validBefore)...) // validBefore
		out = append(out, make([]byte, 32)...)              // nonce
		return out
	}

	t.Run("transferWithAuth decodes the window", func(t *testing.T) {
		t.Parallel()
		input := build([]byte{0xe3, 0xee, 0x16, 0x0e}, 1_700_000_000, 1_700_003_600)
		va, vb, ok := DecodeAuthorizationWindow(input)
		require.True(t, ok)
		require.Equal(t, uint64(1_700_000_000), va.Uint64())
		require.Equal(t, uint64(1_700_003_600), vb.Uint64())
	})

	t.Run("receiveWithAuth decodes the window", func(t *testing.T) {
		t.Parallel()
		input := build([]byte{0xef, 0x55, 0xbe, 0xc6}, 0, 1_700_003_600)
		va, vb, ok := DecodeAuthorizationWindow(input)
		require.True(t, ok)
		require.Equal(t, uint64(0), va.Uint64())
		require.Equal(t, uint64(1_700_003_600), vb.Uint64())
	})

	t.Run("wrapper selector is not decodable", func(t *testing.T) {
		t.Parallel()
		input := build([]byte{0x82, 0xad, 0x56, 0xcb}, 1, 2) // aggregate3
		_, _, ok := DecodeAuthorizationWindow(input)
		require.False(t, ok)
	})

	t.Run("truncated calldata is not decodable", func(t *testing.T) {
		t.Parallel()
		_, _, ok := DecodeAuthorizationWindow([]byte{0xe3, 0xee, 0x16, 0x0e})
		require.False(t, ok)
	})

	t.Run("transferWithAuth bytes-overload decodes the window", func(t *testing.T) {
		t.Parallel()
		input := build([]byte{0xcf, 0x09, 0x29, 0x95}, 1_700_000_000, 1_700_003_600)
		va, vb, ok := DecodeAuthorizationWindow(input)
		require.True(t, ok)
		require.Equal(t, uint64(1_700_000_000), va.Uint64())
		require.Equal(t, uint64(1_700_003_600), vb.Uint64())
	})

	t.Run("decodes the 2^256-1 no-expiry sentinel without truncation", func(t *testing.T) {
		t.Parallel()
		input := append([]byte{0xe3, 0xee, 0x16, 0x0e}, make([]byte, 3*32)...) // selector + from,to,value
		input = append(input, make([]byte, 32)...)                             // validAfter = 0
		input = append(input, bytes.Repeat([]byte{0xff}, 32)...)               // validBefore = 2^256-1
		input = append(input, make([]byte, 32)...)                             // nonce
		_, vb, ok := DecodeAuthorizationWindow(input)
		require.True(t, ok)
		require.Equal(t, 256, vb.BitLen(), "2^256-1 sentinel preserved (would overflow uint64)")
	})
}
