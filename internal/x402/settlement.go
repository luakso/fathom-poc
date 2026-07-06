package x402

import "math/big"

// SettlementKind classifies an x402 settlement by its outer tx selector. It is a
// closed enum — only SettlementTransfer and SettlementReceive are ever produced.
// The string values are the on-the-wire/DB representation (payments.settlement_kind).
type SettlementKind string

const (
	// SettlementTransfer is a transferWithAuthorization or any wrapper
	// (aggregate3, handleOps, charge, …) — the facilitator pushes the payer's
	// authorized funds to the payee.
	SettlementTransfer SettlementKind = "transfer"
	// SettlementReceive is a direct receiveWithAuthorization: a payee-pull,
	// self-settled settlement (msg.sender == payee).
	SettlementReceive SettlementKind = "receive"
)

// ClassifySettlement classifies an x402 settlement by its outer tx selector:
// SettlementReceive for a direct receiveWithAuthorization (payee-pull,
// self-settled), SettlementTransfer for everything else (transferWithAuthorization
// and any wrapper — aggregate3, handleOps, charge, …). The outer selector is the
// only signal inspected; an inner receiveWithAuthorization wrapped in another call
// reads as SettlementTransfer (observed to be ~nonexistent on Base).
//
// (Named ClassifySettlement rather than SettlementKind because the enum TYPE now
// owns that identifier — a type and a func cannot share a name in one package.)
func ClassifySettlement(input []byte) SettlementKind {
	if sig, ok := SighashFromBytes(input); ok {
		if sig == SighashReceiveWithAuthV || sig == SighashReceiveWithAuthB {
			return SettlementReceive
		}
	}
	return SettlementTransfer
}

// DecodeAuthorizationWindow extracts (validAfter, validBefore) from the calldata
// of a direct EIP-3009 entrypoint. Both transferWithAuthorization and
// receiveWithAuthorization (classic + bytes overloads) lay out
// from,to,value,validAfter,validBefore,nonce as the first six 32-byte words
// after the selector. ok is false for wrapper selectors (aggregate3, handleOps,
// …) — their outer calldata is not the EIP-3009 head — or for truncated input.
func DecodeAuthorizationWindow(input []byte) (validAfter, validBefore *big.Int, ok bool) {
	sig, ok := SighashFromBytes(input)
	if !ok {
		return nil, nil, false
	}
	switch sig {
	case SighashTransferWithAuthV, SighashTransferWithAuthB,
		SighashReceiveWithAuthV, SighashReceiveWithAuthB:
	default:
		return nil, nil, false
	}
	const need = 4 + 5*32 // selector + words 0..4 (through validBefore)
	if len(input) < need {
		return nil, nil, false
	}
	validAfter = new(big.Int).SetBytes(input[4+3*32 : 4+4*32])
	validBefore = new(big.Int).SetBytes(input[4+4*32 : 4+5*32])
	return validAfter, validBefore, true
}
