package base

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/lukostrobl/fathom/internal/x402"
)

// ConvertLog turns one HyperSyncLog into an x402.Log. Errors on malformed
// hex inputs; the caller is expected to skip that row and continue the batch.
func ConvertLog(in HyperSyncLog) (x402.Log, error) {
	addr, err := parseAddress(in.Address)
	if err != nil {
		return x402.Log{}, fmt.Errorf("log.address: %w", err)
	}
	topics := make([]common.Hash, 0, len(in.Topics))
	for i, t := range in.Topics {
		h, err := parseHash(t)
		if err != nil {
			return x402.Log{}, fmt.Errorf("log.topics[%d]: %w", i, err)
		}
		topics = append(topics, h)
	}
	data, err := hexBytes(in.Data)
	if err != nil {
		return x402.Log{}, fmt.Errorf("log.data: %w", err)
	}
	txHash, err := parseHash(in.TxHash)
	if err != nil {
		return x402.Log{}, fmt.Errorf("log.tx_hash: %w", err)
	}
	return x402.Log{
		Address:     addr,
		Topics:      topics,
		Data:        data,
		BlockNumber: in.BlockNumber,
		TxHash:      txHash,
		TxIndex:     in.TxIndex,
		LogIndex:    in.LogIndex,
	}, nil
}

// ConvertTransaction turns one HyperSyncTransaction into an x402.Transaction.
func ConvertTransaction(in HyperSyncTransaction) (x402.Transaction, error) {
	hash, err := parseHash(in.Hash)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.hash: %w", err)
	}
	from, err := parseAddress(in.From)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.from: %w", err)
	}
	to, err := parseAddress(in.To)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.to: %w", err)
	}
	input, err := hexBytes(in.Input)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.input: %w", err)
	}
	gasPrice, err := ParseHexInt(in.EffectiveGasPrice)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.effective_gas_price: %w", err)
	}
	nonce, err := parseHexUint64(in.Nonce)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.nonce: %w", err)
	}
	gasUsed, err := parseHexUint64(in.GasUsed)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.gas_used: %w", err)
	}
	// EIP-1559 fee caps are absent on legacy/EIP-2930 txs — leave them nil
	// (→ SQL NULL) rather than coercing an empty string to 0, mirroring the
	// nullable base_fee_per_gas handling in ConvertBlock.
	var maxFee, maxPriorityFee *big.Int
	if in.MaxFeePerGas != "" {
		maxFee, err = ParseHexInt(in.MaxFeePerGas)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.max_fee_per_gas: %w", err)
		}
	}
	if in.MaxPriorityFeePerGas != "" {
		maxPriorityFee, err = ParseHexInt(in.MaxPriorityFeePerGas)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.max_priority_fee_per_gas: %w", err)
		}
	}
	// tx.value is present on most txs (0x0 for token transfers); l1_* are absent
	// on pre-Ecotone / system txs — leave them nil (→ SQL NULL), same pattern as
	// the EIP-1559 fee caps above. ParseHexInt("") returns 0, not nil, so guard
	// each on the empty string.
	var value, l1Fee, l1GasUsed, l1GasPrice *big.Int
	if in.Value != "" {
		value, err = ParseHexInt(in.Value)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.value: %w", err)
		}
	}
	if in.L1Fee != "" {
		l1Fee, err = ParseHexInt(in.L1Fee)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.l1_fee: %w", err)
		}
	}
	if in.L1GasUsed != "" {
		l1GasUsed, err = ParseHexInt(in.L1GasUsed)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.l1_gas_used: %w", err)
		}
	}
	if in.L1GasPrice != "" {
		l1GasPrice, err = ParseHexInt(in.L1GasPrice)
		if err != nil {
			return x402.Transaction{}, fmt.Errorf("tx.l1_gas_price: %w", err)
		}
	}
	// gas is the tx gas LIMIT (distinct from gas_used). parseHexUint64("") == 0
	// covers the theoretical absent case; real txs always carry it.
	gasLimit, err := parseHexUint64(in.Gas)
	if err != nil {
		return x402.Transaction{}, fmt.Errorf("tx.gas: %w", err)
	}
	return x402.Transaction{
		Hash:                 hash,
		BlockNumber:          in.BlockNumber,
		From:                 from,
		To:                   to,
		Input:                input,
		Type:                 in.Type,
		Nonce:                nonce,
		GasUsed:              gasUsed,
		EffectiveGasPrice:    gasPrice,
		MaxFeePerGas:         maxFee,
		MaxPriorityFeePerGas: maxPriorityFee,
		Value:                value,
		GasLimit:             gasLimit,
		L1Fee:                l1Fee,
		L1GasUsed:            l1GasUsed,
		L1GasPrice:           l1GasPrice,
	}, nil
}

// ConvertBlock turns one HyperSyncBlock into an x402.Block. BaseFeePerGas is
// nil for legacy (pre-EIP-1559) blocks that carry no base fee in the wire
// response.
func ConvertBlock(in HyperSyncBlock) (x402.Block, error) {
	hash, err := parseHash(in.Hash)
	if err != nil {
		return x402.Block{}, fmt.Errorf("block.hash: %w", err)
	}
	var baseFee *big.Int
	if in.BaseFeePerGas != "" {
		baseFee, err = ParseHexInt(in.BaseFeePerGas)
		if err != nil {
			return x402.Block{}, fmt.Errorf("block.base_fee_per_gas: %w", err)
		}
	}
	timestamp, err := parseHexUint64(in.Timestamp)
	if err != nil {
		return x402.Block{}, fmt.Errorf("block.timestamp: %w", err)
	}
	return x402.Block{
		Number:        in.Number,
		Timestamp:     timestamp,
		Hash:          hash,
		BaseFeePerGas: baseFee,
	}, nil
}

// parseHexUint64 parses a 0x-prefixed hex quantity into a uint64. HyperSync
// encodes EVM quantity fields (nonce, gas_used, timestamp, …) as hex strings.
// Empty string parses to 0.
func parseHexUint64(s string) (uint64, error) {
	v, err := ParseHexInt(s)
	if err != nil {
		return 0, err
	}
	if !v.IsUint64() {
		return 0, fmt.Errorf("hex value %q overflows uint64", s)
	}
	return v.Uint64(), nil
}

// ParseHexInt parses a 0x-prefixed hex string as a *big.Int.
// Empty string returns 0.
func ParseHexInt(s string) (*big.Int, error) {
	if s == "" {
		return new(big.Int), nil
	}
	v, ok := new(big.Int).SetString(strings.TrimPrefix(s, "0x"), 16)
	if !ok {
		return nil, fmt.Errorf("parse hex int %q", s)
	}
	return v, nil
}

func parseAddress(s string) (common.Address, error) {
	if !strings.HasPrefix(s, "0x") || len(s) != 42 {
		return common.Address{}, fmt.Errorf("invalid address %q", s)
	}
	return common.HexToAddress(s), nil
}

func parseHash(s string) (common.Hash, error) {
	if !strings.HasPrefix(s, "0x") {
		return common.Hash{}, fmt.Errorf("invalid hash %q", s)
	}
	return common.HexToHash(s), nil
}

func hexBytes(s string) ([]byte, error) {
	return hex.DecodeString(strings.TrimPrefix(s, "0x"))
}
