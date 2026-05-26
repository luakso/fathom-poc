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
// checksummed common.Address; callers should lowercase via .Hex() then
// strings.ToLower at the storage boundary (see internal/base.normalizeAddress).
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
)

// 4-byte function selectors (first 4 bytes of keccak256(signature)).
//
// Stored as uint32 for cheap set membership; helper SighashFromBytes converts
// from the leading 4 bytes of tx.input.
const (
	// Verified keccak — see TestSelectorsMatchKeccak.
	SighashTransferWithAuthV uint32 = 0xe3ee160e // classic (v,r,s)
	SighashTransferWithAuthB uint32 = 0xcf092995 // bytes-overload (USDC v2.2)
	SighashReceiveWithAuthV  uint32 = 0xef55bec6 // payee-pull, NOT x402
	SighashReceiveWithAuthB  uint32 = 0x88b7ab63 // payee-pull, NOT x402
	SighashAggregate3        uint32 = 0x82ad56cb // Multicall3.aggregate3

	// Empirical — appears in prior project's data, attribution unconfirmed.
	// See docs/x402-indexing-findings.md Appendix A. The probe subcommand
	// (Plan 4) confirms whether it carries real AuthorizationUsed traffic;
	// if it produces zero rows after probe, remove it from AllowSighashes.
	SighashUnattributedX uint32 = 0x93d9c747
)

// AllowSighashes are outer-tx selectors we keep. Permissive on purpose — the
// AuthorizationUsed-topic match on the USDC proxy is the actual filter (see
// findings §6.4). Widening the allow-list lets some non-x402 calldata through
// the sighash gate, but it never emits the event so it never becomes a row.
var AllowSighashes = []uint32{
	SighashTransferWithAuthV,
	SighashTransferWithAuthB,
	SighashAggregate3,
	SighashUnattributedX,
}

// DenySighashes are outer-tx selectors we explicitly reject. receiveWithAuthorization
// is payee-pull (no facilitator) and emits the same AuthorizationUsed event as
// transferWithAuthorization — without this exclude, we'd misclassify those rows.
var DenySighashes = []uint32{
	SighashReceiveWithAuthV,
	SighashReceiveWithAuthB,
}

// USDCDecimals is the number of decimal places USDC uses.
// 1 USDC = 10^6 base units.
const USDCDecimals = 6

// ChainBase is the canonical chain identifier stored in payments.chain.
const ChainBase = "base"
