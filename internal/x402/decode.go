package x402

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// Log is a chain-agnostic event log shape. Both the HyperSync stream and
// go-ethereum's types.Log fields project into this; converters in Plans 2/3
// adapt their respective wire types.
type Log struct {
	Address     common.Address
	Topics      []common.Hash
	Data        []byte
	BlockNumber uint64
	TxHash      common.Hash
	TxIndex     uint32
	LogIndex    uint32
}

// Transaction is a chain-agnostic transaction shape — same idea as Log.
type Transaction struct {
	Hash              common.Hash
	BlockNumber       uint64
	From              common.Address // facilitator for x402
	To                common.Address // USDC, Multicall3, or wrapper
	Input             []byte         // calldata; Input[0:4] = sighash
	Type              uint8
	Nonce             uint64
	GasUsed           uint64
	EffectiveGasPrice *big.Int
	BaseFeePerGas     *big.Int
}

// Block carries only the per-block context we actually use.
type Block struct {
	Number    uint64
	Timestamp uint64 // unix seconds
	Hash      common.Hash
}

// DecodeTransfer extracts (from, to, value) from a standard ERC-20 Transfer log.
// Caller is responsible for confirming Topics[0] == TransferTopic and
// Address == USDCProxyBase before calling.
func DecodeTransfer(log Log) (from, to common.Address, value *big.Int, err error) {
	if len(log.Topics) != 3 {
		return common.Address{}, common.Address{}, nil,
			fmt.Errorf("transfer log: expected 3 topics, got %d", len(log.Topics))
	}
	if len(log.Data) < 32 {
		return common.Address{}, common.Address{}, nil,
			fmt.Errorf("transfer log: data too short (%d bytes)", len(log.Data))
	}
	from = common.BytesToAddress(log.Topics[1].Bytes()) // last 20 bytes of the 32-byte topic
	to = common.BytesToAddress(log.Topics[2].Bytes())
	value = new(big.Int).SetBytes(log.Data[:32])
	return from, to, value, nil
}

// DecodeAuthorizationUsed extracts (authorizer, nonce) from an EIP-3009
// AuthorizationUsed log. Caller is responsible for confirming
// Topics[0] == AuthorizationUsedTopic and Address == USDCProxyBase.
//
// Returned nonce is a copy — safe to store.
func DecodeAuthorizationUsed(log Log) (authorizer common.Address, nonce []byte, err error) {
	if len(log.Topics) != 2 {
		return common.Address{}, nil,
			fmt.Errorf("authorization-used log: expected 2 topics, got %d", len(log.Topics))
	}
	if len(log.Data) < 32 {
		return common.Address{}, nil,
			fmt.Errorf("authorization-used log: data too short (%d bytes)", len(log.Data))
	}
	authorizer = common.BytesToAddress(log.Topics[1].Bytes())
	nonce = make([]byte, 32)
	copy(nonce, log.Data[:32])
	return authorizer, nonce, nil
}

// SighashFromBytes extracts the leading 4 bytes of calldata as a uint32. The
// boolean is false when the input is shorter than 4 bytes (a contract creation
// or empty calldata).
func SighashFromBytes(input []byte) (uint32, bool) {
	if len(input) < 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(input[:4]), true
}
