package x402

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettlementKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input []byte
		want  string
	}{
		{"transferWithAuth classic", []byte{0xe3, 0xee, 0x16, 0x0e}, "transfer"},
		{"transferWithAuth bytes", []byte{0xcf, 0x09, 0x29, 0x95}, "transfer"},
		{"receiveWithAuth classic", []byte{0xef, 0x55, 0xbe, 0xc6}, "receive"},
		{"receiveWithAuth bytes", []byte{0x88, 0xb7, 0xab, 0x63}, "receive"},
		{"aggregate3 wrapper", []byte{0x82, 0xad, 0x56, 0xcb}, "transfer"},
		{"no selector", []byte{0x00}, "transfer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, c.want, SettlementKind(c.input))
		})
	}
}
