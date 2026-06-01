package x402

// KeepAuthorizationUsed implements the topic-only keep policy: an
// AuthorizationUsed log on the USDC proxy IS an x402 payment, with one
// exception — a *direct* receiveWithAuthorization call (payee-pull, not x402),
// which emits the same event and is excluded by its outer selector.
//
// We deliberately do NOT gate on an allow-list of outer selectors. Real
// settlements arrive wrapped in many shapes (ERC-4337 handleOps,
// batchTransferWithAuthorization, charge, settle, payWithSignature, …); an
// allow-list silently dropped ~4.5% of genuine payments (see the probe
// coverage-gap analysis). The AuthorizationUsed-on-USDC topic is the gate; the
// deny-list is the only selector-based carve-out.
//
// Caveat: the deny carve-out inspects only the *outer* selector. A
// receiveWithAuthorization wrapped inside another call (e.g. handleOps) is not
// caught here — observed to be ~nonexistent on Base, but if that changes the
// fix is an inner-call decode (the "c-robust" option).
func KeepAuthorizationUsed(authLog Log, parentTxInput []byte) bool {
	if authLog.Address != USDCProxyBase {
		return false
	}
	if len(authLog.Topics) == 0 || authLog.Topics[0] != AuthorizationUsedTopic {
		return false
	}
	// A parseable 4-byte selector is required: downstream row-building records it
	// (Payment.MethodSelector reads parentTxInput[:4]), and an input too short to
	// carry a selector cannot be a real USDC settlement. Then exclude only a
	// direct receiveWithAuthorization (payee-pull); everything else is kept.
	sighash, ok := SighashFromBytes(parentTxInput)
	if !ok {
		return false
	}
	if containsUint32(DenySighashes, sighash) {
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
