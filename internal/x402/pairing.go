package x402

// PairCompanionTransfer finds the USDC Transfer log that pairs with the
// AuthorizationUsed log at authLogIndex.
//
// The rule (findings §4): for an AuthorizationUsed at log_index = K, return
// the USDC Transfer with the HIGHEST log_index strictly less than K. The
// intuitive "nearest by absolute distance" rule silently corrupts multicalls
// — a Transfer after the auth belongs to a later settlement, never to this one.
//
// receiptLogs must be the full ordered log slice from the receipt; the
// function does not assume any particular ordering and tolerates non-USDC and
// non-Transfer entries mixed in.
//
// Returns (Log{}, false) if no qualifying Transfer is present.
func PairCompanionTransfer(receiptLogs []Log, authLogIndex uint32) (Log, bool) {
	var best Log
	found := false
	for _, lg := range receiptLogs {
		if lg.Address != USDCProxyBase {
			continue
		}
		if len(lg.Topics) == 0 || lg.Topics[0] != TransferTopic {
			continue
		}
		if lg.LogIndex >= authLogIndex {
			continue // must strictly precede the auth
		}
		if !found || lg.LogIndex > best.LogIndex {
			best = lg
			found = true
		}
	}
	return best, found
}
