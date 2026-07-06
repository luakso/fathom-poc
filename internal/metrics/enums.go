package metrics

// This file defines the package's closed enum vocabularies as named string
// types. They replace bare string literals for membership, window, role, and
// claim kind across the presentation layer.
//
// WIRE COMPATIBILITY: a named string type marshals to JSON identically to the
// underlying string, and encoding/json accepts a named-string map key exactly
// like a string key. So typing struct fields and output map keys with these
// enums leaves the emitted artifacts byte-for-byte unchanged. SQL string
// literals stay as-is; values are converted at the Go boundary (scan / index).

// Membership is the verified-vs-excluded split derived from facilitator_known.
type Membership string

const (
	// MembershipKnown is a verified x402 payment (facilitator_known = true).
	MembershipKnown Membership = "known"
	// MembershipUnknown is the excluded remainder (facilitator_known = false).
	MembershipUnknown Membership = "unknown"
	// MembershipAll is the synthetic union used by window-stats GROUPING SETS.
	MembershipAll Membership = "all"
)

// Window is one of the fixed emit lookback windows.
type Window string

const (
	// Window7d is the trailing 7 days (asOf minus 6, inclusive).
	Window7d Window = "7d"
	// Window30d is the trailing 30 days (asOf minus 29, inclusive).
	Window30d Window = "30d"
	// WindowAll is the full history (no lower bound).
	WindowAll Window = "all"
)

// Role is an entity's side of a payment.
type Role string

const (
	// RolePayee is the receiving side.
	RolePayee Role = "payee"
	// RolePayer is the paying side.
	RolePayer Role = "payer"
)

// ClaimKind is the measured dimension a curated claim compares against.
type ClaimKind string

const (
	// ClaimKindTxns compares transaction counts.
	ClaimKindTxns ClaimKind = "txns"
	// ClaimKindVolume compares USDC volume.
	ClaimKindVolume ClaimKind = "volume"
)
