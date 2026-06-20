package anatomy

import (
	"encoding/hex"
	"strings"
)

// StatusSuccess is the only status Anatomy reports: the substrate indexes only
// settled payment logs, so a tx present here necessarily succeeded.
const StatusSuccess = "success"

type methodInfo struct{ name, kind string }

// knownMethods maps a 4-byte selector (hex) to its decoded name and a short
// "kind" qualifier. The two transferWithAuthorization entries are the EIP-3009
// (v,r,s) and (bytes signature) overloads.
var knownMethods = map[string]methodInfo{
	"e3ee160e": {"transferWithAuthorization", "v,r,s"},
	"cf092995": {"transferWithAuthorization", "sig"},
	"82ad56cb": {"aggregate3", "multicall"},
	"1fad948c": {"handleOps", "erc-4337"},
}

// MethodName decodes a transaction's method selector. For unknown selectors it
// returns the raw "0x…" hex as the name with known=false.
func MethodName(selector []byte) (name, kind string, known bool) {
	h := hex.EncodeToString(selector)
	if m, ok := knownMethods[h]; ok {
		return m.name, m.kind, true
	}
	return "0x" + h, "", false
}

// knownContracts maps a lowercased "chain|address" to a human label.
var knownContracts = map[string]string{
	"base|0x833589fcd6edb6e08f4c7c32d4f71b54bda02913": "USDC · Circle",
	"base|0xca11bde05977b3631167028862be2a173976ca11": "Multicall3",
	"base|0x5ff137d4b0fdcd49dca30c7cf57e578a026d2789": "EntryPoint (ERC-4337)",
}

// ContractLabel returns a human label for a known contract, or "" if unknown.
func ContractLabel(chain, addr string) string {
	return knownContracts[strings.ToLower(chain)+"|"+strings.ToLower(addr)]
}

// ExplorerTxURL returns the block-explorer URL for a tx, or "" for unknown chains.
func ExplorerTxURL(chain, hash string) string {
	switch chain {
	case "base":
		return "https://basescan.org/tx/" + hash
	case "solana":
		return "https://solscan.io/tx/" + hash
	default:
		return ""
	}
}
