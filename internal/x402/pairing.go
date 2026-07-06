package x402

// PairCompanionTransfer finds the USDC Transfer log that pairs with the
// AuthorizationUsed log at authLogIndex.
//
// The rule: for an AuthorizationUsed at log_index = K, return the USDC Transfer
// with the LOWEST log_index strictly greater than K. USDC's EIP-3009
// _transferWithAuthorization emits AuthorizationUsed (in _markAuthorizationAsUsed)
// BEFORE the Transfer (in _transfer), so the companion always immediately
// FOLLOWS the auth. Verified on-chain: 221/221 single-pair transferWithAuthorization
// txs show AUTH at K, Transfer at K+1. In a multicall [AUTH0,XFER0,AUTH1,XFER1,…],
// each auth pairs with the next transfer; a Transfer before the auth belongs to
// an earlier payment, never to this one. The authorizer==Transfer.from check in
// Assemble is the backstop against mis-pairing.
//
// receiptLogs must be the full ordered log slice from the receipt; the
// function does not assume any particular ordering and tolerates non-USDC and
// non-Transfer entries mixed in.
//
// consumed is the set of USDC Transfer log indices already bound to an earlier
// auth in the SAME receipt; those are skipped so one Transfer never backs two
// payments (see Assemble). Pass nil (or an empty map) when nothing is consumed.
//
// Returns (Log{}, false) if no qualifying, unconsumed Transfer is present.
func PairCompanionTransfer(receiptLogs []Log, authLogIndex uint32, consumed map[uint32]bool) (Log, bool) {
	var best Log
	found := false
	for _, lg := range receiptLogs {
		if lg.Address != USDCProxyBase {
			continue
		}
		if len(lg.Topics) == 0 || lg.Topics[0] != TransferTopic {
			continue
		}
		if lg.LogIndex <= authLogIndex {
			continue // companion Transfer must strictly follow the auth
		}
		if consumed[lg.LogIndex] {
			continue // already paired to an earlier auth in this receipt
		}
		if !found || lg.LogIndex < best.LogIndex {
			best = lg
			found = true
		}
	}
	return best, found
}
