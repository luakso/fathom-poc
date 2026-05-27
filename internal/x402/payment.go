package x402

import (
	"math/big"
	"time"

	"github.com/shopspring/decimal"
)

// Payment is one observed x402 settlement row. Plain struct on purpose —
// decoders fill it, the Store persists it. Every field maps 1:1 to a column in
// the payments table (see database/migrations/00002_payments.sql).
//
// Address fields are stored lowercased; the Assemble step normalizes.
// AmountRaw is the canonical u256 base units (NUMERIC(78,0) on the wire),
// AmountUSDC is the convenience derivation amount_raw / 10^USDCDecimals.
// Never aggregate AmountUSDC — sum AmountRaw and divide once (findings §11).
type Payment struct {
	// Identity / PK
	Chain    string // 'base'
	TxHash   string // lowercased hex with 0x prefix
	LogIndex uint32 // AuthorizationUsed log index in the receipt

	// Position on chain
	BlockNumber    uint64
	BlockTimestamp time.Time // UTC

	// Observation metadata
	Source   string // 'base-collector'
	Protocol string // 'x402'

	// Payment principals (all lowercased addresses)
	Facilitator    string // tx.from
	Payer          string // EIP-3009 authorizer == Transfer.from
	Payee          string // Transfer.to
	PayeeServiceID *int64 // logical link, may be nil

	// Amount (exact + convenience; never lose precision)
	Asset          string
	TokenAddress   string          // USDC proxy address, lowercased
	AmountRaw      *big.Int        // u256 base units
	AmountUSDC     decimal.Decimal // AmountRaw / 10^USDCDecimals
	AssetUSDAtTime decimal.Decimal // 1.0 for USDC in v1

	// EIP-3009 authorization id (bytes32). Named distinct from TxNonce.
	AuthNonce []byte

	// Routing metadata
	MethodSelector []byte // first 4 bytes of tx.input
	CalledContract string // tx.to, lowercased
	TxType         uint8  // 0=legacy 1=EIP-2930 2=EIP-1559 3=EIP-4844
	TxNonce        uint64 // tx.from's sequence number — NOT AuthNonce

	// Gas economics
	GasUsed           uint64
	EffectiveGasPrice *big.Int // wei
	GasCostWei        *big.Int // GasUsed * EffectiveGasPrice
	BaseFeePerGas     *big.Int // nullable on legacy txs
}

// USDCFromRaw converts a USDC base-unit amount to a six-decimal decimal.
// The conversion is exact (no f64). Callers should still aggregate from
// AmountRaw and divide once for accounting use cases.
func USDCFromRaw(raw *big.Int) decimal.Decimal {
	if raw == nil {
		return decimal.Zero
	}
	return decimal.NewFromBigInt(raw, -USDCDecimals)
}
