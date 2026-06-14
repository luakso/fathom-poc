package x402

// KeepAuthorizationUsed implements the v2 topic-only keep policy: an
// AuthorizationUsed log on the USDC proxy IS an x402 payment as long as its
// parent tx carries a parseable 4-byte selector. receiveWithAuthorization is no
// longer denied — it is captured and flagged settlement_kind='receive' /
// self_settled=true downstream (see x402.SettlementKind and the membership
// ladder in docs/superpowers/specs/2026-06-14-x402-entity-substrate-design.md).
// Confirmed-non-x402 contracts (ERC-4337 EntryPoint, batch utils) are excluded
// later, by the contamination denylist, not here.
func KeepAuthorizationUsed(authLog Log, parentTxInput []byte) bool {
	if authLog.Address != USDCProxyBase {
		return false
	}
	if len(authLog.Topics) == 0 || authLog.Topics[0] != AuthorizationUsedTopic {
		return false
	}
	// A parseable 4-byte selector is still required: row-building records it
	// (Payment.MethodSelector reads parentTxInput[:4]) and an input too short to
	// carry a selector cannot be a real USDC settlement.
	if _, ok := SighashFromBytes(parentTxInput); !ok {
		return false
	}
	return true
}
