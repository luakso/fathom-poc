package x402

// SettlementKind classifies an x402 settlement by its outer tx selector:
// "receive" for a direct receiveWithAuthorization (payee-pull, self-settled),
// "transfer" for everything else (transferWithAuthorization and any wrapper —
// aggregate3, handleOps, charge, …). The outer selector is the only signal
// inspected; an inner receiveWithAuthorization wrapped in another call reads as
// "transfer" (observed to be ~nonexistent on Base).
func SettlementKind(input []byte) string {
	if sig, ok := SighashFromBytes(input); ok {
		if sig == SighashReceiveWithAuthV || sig == SighashReceiveWithAuthB {
			return "receive"
		}
	}
	return "transfer"
}
