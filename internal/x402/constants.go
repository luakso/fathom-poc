// Package x402 holds chain-side primitives for the x402 protocol:
// canonical addresses, event topics, function selectors, ABI decoders,
// companion-Transfer pairing, and the filter predicate.
//
// Every named hex constant in this file is verified against
// keccak256(signature)[0..4] by constants_test.go. The lone exception is
// SighashUnattributedX (0x93d9c747), which is empirical — its signature is
// unknown but it appears in observed Base mainnet data emitting the right
// AuthorizationUsed events. See docs/x402-indexing-findings.md §13 and
// docs/superpowers/specs/2026-05-22-base-collector-design.md §5.
package x402

import "github.com/ethereum/go-ethereum/common"

// Canonical addresses on Base mainnet (chain ID 8453). Stored as the
// checksummed common.Address; callers should lowercase via strings.ToLower(addr.Hex())
// at the storage boundary (see internal/x402.Assemble).
var (
	// USDCProxyBase is the Circle USDC proxy on Base. All x402 EIP-3009
	// settlements emit AuthorizationUsed from this address.
	USDCProxyBase = common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")

	// Multicall3 is the canonical Multicall3 deployment, same address on every
	// EVM chain. Used here so we can attribute aggregate3-wrapped settlements.
	Multicall3 = common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11")
)

// Event topic hashes (keccak256 of the canonical event signature).
var (
	// AuthorizationUsedTopic is emitted by USDC when an EIP-3009 authorization
	// is consumed. This is the final-gate filter — every payments row must have
	// one of these in its parent receipt on the USDC proxy.
	AuthorizationUsedTopic = common.HexToHash("0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5")

	// TransferTopic is the standard ERC-20 Transfer event. Companion to
	// AuthorizationUsed for amount + recipient pairing (see pairing.go).
	TransferTopic = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	// AuthorizationCanceledTopic is emitted by USDC when an EIP-3009
	// authorization nonce is canceled via cancelAuthorization — the payer
	// abandoned a signed authorization before it was ever used. Same USDC
	// proxy as AuthorizationUsed, different topic0. Both args indexed:
	// topics[1] = authorizer, topics[2] = nonce, empty data.
	AuthorizationCanceledTopic = common.HexToHash("0x1cdd46ff242716cdaa72d159d339a485b3438398348d68f09d7c8c0a59353d81")
)

// 4-byte function selectors (first 4 bytes of keccak256(signature)).
//
// Stored as uint32 for cheap set membership; helper SighashFromBytes converts
// from the leading 4 bytes of tx.input.
const (
	// Verified keccak — see TestSelectorsMatchKeccak.
	SighashTransferWithAuthV uint32 = 0xe3ee160e // classic (v,r,s)
	SighashTransferWithAuthB uint32 = 0xcf092995 // bytes-overload (USDC v2.2)
	SighashReceiveWithAuthV  uint32 = 0xef55bec6 // payee-pull, self-settled x402 (settlement_kind='receive')
	SighashReceiveWithAuthB  uint32 = 0x88b7ab63 // payee-pull, self-settled x402 (settlement_kind='receive')
	SighashAggregate3        uint32 = 0x82ad56cb // Multicall3.aggregate3

	// Empirical — appears in observed Base data; 4byte attributes it to
	// settleAndExecute(...), attribution still unconfirmed. The probe confirmed
	// it carries real AuthorizationUsed traffic (thousands of events), so it is
	// a real x402 settlement path. Kept here only as a HyperSync query hint; the
	// client keep-policy is topic-only (see filter.go) and no longer depends on
	// this list. See docs/x402-indexing-findings.md Appendix A.
	SighashUnattributedX uint32 = 0x93d9c747
)

// AllowSighashes is NOT a keep-filter and is no longer used to build the
// HyperSync query (the vestigial transaction hint was removed — selection is
// log-only). The client keep-policy is topic-only (see KeepAuthorizationUsed):
// every AuthorizationUsed-on-USDC log with a parseable 4-byte selector is a
// payment — no selector-based carve-outs remain (receiveWithAuthorization is
// captured and flagged settlement_kind='receive'). This list is retained as the documented set of
// known x402 outer selectors and as the disjointness anchor for
// TestAllowDenyDisjoint against DenySighashes.
var AllowSighashes = []uint32{
	SighashTransferWithAuthV,
	SighashTransferWithAuthB,
	SighashAggregate3,
	SighashUnattributedX,
}

// DenySighashes are the direct receiveWithAuthorization selectors. They are NO
// LONGER rejected by the filter (v2 captures them flagged settlement_kind=
// 'receive'); the list is retained as the disjointness anchor for
// TestAllowDenyDisjoint and as the documented receive-classification set.
var DenySighashes = []uint32{
	SighashReceiveWithAuthV,
	SighashReceiveWithAuthB,
}

// USDCDecimals is the number of decimal places USDC uses.
// 1 USDC = 10^6 base units.
const USDCDecimals = 6

// ChainBase is the canonical chain identifier stored in payments.chain.
const ChainBase = "base"
