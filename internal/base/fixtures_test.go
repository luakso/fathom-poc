package base_test

// Shared, DB-free test fixtures for the base package. Lives in an untagged file
// (no `integration` build tag) so both the fast unit suite (probe_test.go) and
// the integration suite (backfill_test.go etc.) can use them. DB-backed helpers
// like setupStore stay in their integration-tagged files.

import (
	"context"
	"strings"

	"github.com/lukostrobl/fathom/internal/base"
	"github.com/lukostrobl/fathom/internal/x402"
)

// fakeFetcher returns a fixed sequence of batches.
type fakeFetcher struct{ batches []base.HyperSyncBatch }

func (f *fakeFetcher) Stream(_ context.Context, _ base.HyperSyncQuery) (base.Stream, error) {
	return &fakeStream{batches: f.batches}, nil
}

type fakeStream struct {
	batches []base.HyperSyncBatch
	idx     int
}

func (s *fakeStream) Next(_ context.Context) (base.HyperSyncBatch, bool, error) {
	if s.idx >= len(s.batches) {
		return base.HyperSyncBatch{}, false, nil
	}
	b := s.batches[s.idx]
	s.idx++
	return b, true, nil
}
func (s *fakeStream) Close() error { return nil }

// fixtureBatch builds a HyperSyncBatch representing one classic-sig
// transferWithAuthorization in block 100.
func fixtureBatch() base.HyperSyncBatch {
	payer := "0x000000000000000000000000aaaa000000000000000000000000000000000001"
	payee := "0x000000000000000000000000bbbb000000000000000000000000000000000001"
	return base.HyperSyncBatch{
		Data: base.HyperSyncBatchData{
			Logs: []base.HyperSyncLog{
				// Real USDC EIP-3009 order: AuthorizationUsed (log_index 0) first.
				// Both params indexed → 3 topics (sig, authorizer, nonce), empty data.
				{
					Address: strings.ToLower(x402.USDCProxyBase.Hex()),
					Topics: []string{
						x402.AuthorizationUsedTopic.Hex(),
						payer,
						"0x1111111111111111111111111111111111111111111111111111111111111111", // nonce (indexed)
					},
					Data:        "0x",
					BlockNumber: 100,
					TxHash:      "0xdead",
					TxIndex:     0,
					LogIndex:    0,
				},
				// ...then the companion Transfer (log_index 1).
				{
					Address: strings.ToLower(x402.USDCProxyBase.Hex()),
					Topics: []string{
						x402.TransferTopic.Hex(),
						payer, payee,
					},
					Data:        "0x00000000000000000000000000000000000000000000000000000000000f4240", // 1 USDC
					BlockNumber: 100,
					TxHash:      "0xdead",
					TxIndex:     0,
					LogIndex:    1,
				},
			},
			Transactions: []base.HyperSyncTransaction{
				{
					Hash:              "0xdead",
					BlockNumber:       100,
					From:              "0xfac1000000000000000000000000000000000001",
					To:                strings.ToLower(x402.USDCProxyBase.Hex()),
					Input:             "0xe3ee160edeadbeef",
					Type:              2,
					Nonce:             "0x7",    // 7
					GasUsed:           "0xc350", // 50_000
					EffectiveGasPrice: "0x3b9aca00",
				},
			},
			Blocks: []base.HyperSyncBlock{
				{Number: 100, Timestamp: "0x6553f100", Hash: "0xb100", BaseFeePerGas: "0x1dcd6500"}, // ts 1_700_000_000
			},
		},
		NextBlock: 101,
	}
}
