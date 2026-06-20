package anatomy

import "testing"

func TestMethodName(t *testing.T) {
	cases := []struct {
		sel        []byte
		name, kind string
		known      bool
	}{
		{[]byte{0xe3, 0xee, 0x16, 0x0e}, "transferWithAuthorization", "v,r,s", true},
		{[]byte{0xcf, 0x09, 0x29, 0x95}, "transferWithAuthorization", "sig", true},
		{[]byte{0x82, 0xad, 0x56, 0xcb}, "aggregate3", "multicall", true},
		{[]byte{0x1f, 0xad, 0x94, 0x8c}, "handleOps", "erc-4337", true},
		{[]byte{0xde, 0xad, 0xbe, 0xef}, "0xdeadbeef", "", false},
		{nil, "0x", "", false},
	}
	for _, c := range cases {
		name, kind, known := MethodName(c.sel)
		if name != c.name || kind != c.kind || known != c.known {
			t.Errorf("MethodName(%x) = (%q,%q,%v), want (%q,%q,%v)",
				c.sel, name, kind, known, c.name, c.kind, c.known)
		}
	}
}

func TestContractLabel(t *testing.T) {
	if got := ContractLabel("base", "0x833589FCD6EDB6E08F4C7C32D4F71B54BDA02913"); got != "USDC · Circle" {
		t.Errorf("USDC label = %q", got)
	}
	if got := ContractLabel("base", "0xca11bde05977b3631167028862be2a173976ca11"); got != "Multicall3" {
		t.Errorf("multicall label = %q", got)
	}
	if got := ContractLabel("base", "0xunknown"); got != "" {
		t.Errorf("unknown label = %q, want empty", got)
	}
}

func TestExplorerTxURL(t *testing.T) {
	if got := ExplorerTxURL("base", "0xabc"); got != "https://basescan.org/tx/0xabc" {
		t.Errorf("base url = %q", got)
	}
	if got := ExplorerTxURL("solana", "sig123"); got != "https://solscan.io/tx/sig123" {
		t.Errorf("solana url = %q", got)
	}
	if got := ExplorerTxURL("unknown", "x"); got != "" {
		t.Errorf("unknown chain url = %q, want empty", got)
	}
}
