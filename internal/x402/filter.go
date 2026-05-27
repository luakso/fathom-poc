package x402

// KeepAuthorizationUsed returns true when an AuthorizationUsed log on the USDC
// proxy is paired with a parent transaction whose calldata selector is in the
// allow-list and not in the deny-list.
//
// Per the spec (§5), the topic+address check is the final gate. The sighash
// allow-list is permissive on purpose: widening it lets some non-x402
// calldata through, but those transactions never emit AuthorizationUsed on
// the USDC proxy, so the topic match keeps them out anyway.
func KeepAuthorizationUsed(authLog Log, parentTxInput []byte) bool {
	if authLog.Address != USDCProxyBase {
		return false
	}
	if len(authLog.Topics) == 0 || authLog.Topics[0] != AuthorizationUsedTopic {
		return false
	}
	sighash, ok := SighashFromBytes(parentTxInput)
	if !ok {
		return false
	}
	if containsUint32(DenySighashes, sighash) {
		return false
	}
	if !containsUint32(AllowSighashes, sighash) {
		return false
	}
	return true
}

func containsUint32(haystack []uint32, needle uint32) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
