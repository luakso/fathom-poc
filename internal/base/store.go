// Package base owns Postgres persistence for base-collector. The Store is the
// sole consumer of pgx in this binary; Backfill calls Store.InsertBatch with a
// []x402.Payment slice plus the max block observed in the batch. Inserts +
// cursor advance happen in one transaction.
package base

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/lukostrobl/fathom/internal/x402"
)

const collectorName = "base-collector"

// Store wraps a pgxpool.Pool with the operations base-collector needs.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store. The pool's lifetime is owned by the caller.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// InsertBatch inserts every row in batch and advances the cursor to
// maxBlock, all in one Postgres transaction. If any insert fails, the whole
// transaction rolls back — neither the rows nor the cursor advance.
//
// Idempotent: ON CONFLICT (chain, tx_hash, log_index) DO NOTHING absorbs
// re-runs over the same range. The cursor advance is monotonic — a smaller
// maxBlock than the current cursor is a no-op.
//
// maxBlock == 0 skips the cursor advance entirely. This is the §7
// empty-batch guard: HyperSync can deliver a batch with no logs and
// max_block = 0, and blindly writing it would reset progress to genesis.
func (s *Store) InsertBatch(ctx context.Context, batch []x402.Payment, maxBlock uint64) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	for i := range batch {
		if err := insertPayment(ctx, tx, &batch[i]); err != nil {
			return fmt.Errorf("insert payment[%d] tx=%s log_index=%d: %w",
				i, batch[i].TxHash, batch[i].LogIndex, err)
		}
	}

	if maxBlock > 0 {
		if err := advanceCursor(ctx, tx, maxBlock); err != nil {
			return fmt.Errorf("advance cursor: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Pool returns the underlying pgxpool.Pool. Provided for tests and the
// status subcommand (Plan 4) which runs ad-hoc aggregations. Production
// code should prefer the typed methods on Store.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// GetCursor returns the last fully-committed block for this collector. Returns
// 0 if no cursor row exists yet (the "never synced" state).
func (s *Store) GetCursor(ctx context.Context) (uint64, error) {
	var last int64
	err := s.pool.QueryRow(
		ctx,
		`SELECT last_block FROM collector_cursor WHERE collector_name = $1`,
		collectorName,
	).Scan(&last)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read cursor: %w", err)
	}
	if last < 0 {
		return 0, fmt.Errorf("cursor unexpectedly negative: %d", last)
	}
	return uint64(last), nil
}

func insertPayment(ctx context.Context, tx pgx.Tx, p *x402.Payment) error {
	amountRaw := decimal.NewFromBigInt(p.AmountRaw, 0)
	effectiveGasPrice := decimal.NewFromBigInt(p.EffectiveGasPrice, 0)
	gasCostWei := decimal.NewFromBigInt(p.GasCostWei, 0)
	var baseFee any
	if p.BaseFeePerGas != nil {
		baseFee = decimal.NewFromBigInt(p.BaseFeePerGas, 0)
	}

	_, err := tx.Exec(
		ctx, `
        INSERT INTO payments (
            chain, tx_hash, log_index,
            block_number, block_timestamp,
            source, protocol,
            facilitator, payer, payee, payee_service_id,
            asset, token_address, amount_raw, amount_usdc, asset_usd_at_time,
            auth_nonce,
            method_selector, called_contract, tx_type, tx_nonce,
            gas_used, effective_gas_price, gas_cost_wei, base_fee_per_gas
        )
        VALUES (
            $1, $2, $3,
            $4, $5,
            $6, $7,
            $8, $9, $10, $11,
            $12, $13, $14, $15, $16,
            $17,
            $18, $19, $20, $21,
            $22, $23, $24, $25
        )
        ON CONFLICT (chain, tx_hash, log_index) DO NOTHING
    `,
		p.Chain, p.TxHash, int32(p.LogIndex), //nolint:gosec // log_index fits in int32; receipts cap well below 2^31
		int64(p.BlockNumber), p.BlockTimestamp, //nolint:gosec // Base block numbers will never approach 2^63
		p.Source, p.Protocol,
		p.Facilitator, p.Payer, p.Payee, p.PayeeServiceID,
		p.Asset, p.TokenAddress, amountRaw, p.AmountUSDC, p.AssetUSDAtTime,
		p.AuthNonce,
		p.MethodSelector, p.CalledContract, int16(p.TxType), int64(p.TxNonce), //nolint:gosec // tx_nonce fits in int64 for centuries
		int64(p.GasUsed), effectiveGasPrice, gasCostWei, baseFee, //nolint:gosec // gas_used realistic blocks << 2^63
	)
	if err != nil {
		return err
	}
	return nil
}

func advanceCursor(ctx context.Context, tx pgx.Tx, newBlock uint64) error {
	_, err := tx.Exec(ctx, `
        INSERT INTO collector_cursor (collector_name, last_block, updated_at)
        VALUES ($1, $2, now())
        ON CONFLICT (collector_name)
        DO UPDATE SET
            last_block = EXCLUDED.last_block,
            updated_at = now()
        WHERE collector_cursor.last_block < EXCLUDED.last_block
    `, collectorName, int64(newBlock)) //nolint:gosec // Base block numbers will never approach 2^63
	return err
}
