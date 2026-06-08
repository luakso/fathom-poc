// Package base owns Postgres persistence for base-collector. The Store is the
// sole consumer of pgx in this binary; Backfill calls Store.InsertBatch with a
// []x402.Payment slice plus the max block observed in the batch. Inserts +
// cursor advance happen in one transaction.
package base

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukostrobl/fathom/internal/x402"
)

const collectorName = "base-collector"

// stageTable is the per-transaction staging table COPY bulk-loads into before
// the deduped INSERT … SELECT. ON COMMIT DROP ties its lifetime to the batch
// transaction, so each InsertBatch gets a fresh, empty staging table.
const stageTable = "payments_copy_stage"

// copyColumns is the column order shared by the COPY into the staging table and
// the INSERT … SELECT out of it. observed_at is intentionally absent so it
// keeps its DEFAULT now() — matching the row-by-row path it replaces.
var copyColumns = []string{
	"chain", "tx_hash", "log_index",
	"block_number", "block_timestamp",
	"source", "protocol",
	"facilitator", "payer", "payee", "payee_service_id",
	"asset", "token_address", "amount_raw", "amount_usdc", "asset_usd_at_time",
	"auth_nonce",
	"method_selector", "called_contract", "tx_type", "tx_nonce",
	"gas_used", "effective_gas_price", "gas_cost_wei", "base_fee_per_gas",
	"max_fee_per_gas", "max_priority_fee_per_gas",
}

// createStageTable types the six NUMERIC columns (amount_raw, amount_usdc,
// asset_usd_at_time, effective_gas_price, gas_cost_wei, base_fee_per_gas) as
// TEXT. pgx's COPY uses the binary wire format and there is no shopspring
// decimal codec registered on the pool, so decimals cannot be binary-encoded
// into NUMERIC directly. Staging them as TEXT and casting ::numeric on the
// INSERT … SELECT keeps full precision without a codec dependency.
const createStageTable = `
CREATE TEMP TABLE ` + stageTable + ` (
    chain               text,
    tx_hash             text,
    log_index           integer,
    block_number        bigint,
    block_timestamp     timestamptz,
    source              text,
    protocol            text,
    facilitator         text,
    payer               text,
    payee               text,
    payee_service_id    bigint,
    asset               text,
    token_address       text,
    amount_raw          text,
    amount_usdc         text,
    asset_usd_at_time   text,
    auth_nonce          bytea,
    method_selector     bytea,
    called_contract     text,
    tx_type             smallint,
    tx_nonce            bigint,
    gas_used            bigint,
    effective_gas_price text,
    gas_cost_wei        text,
    base_fee_per_gas    text,
    max_fee_per_gas          text,
    max_priority_fee_per_gas text
) ON COMMIT DROP`

// insertFromStage moves staged rows into payments with the same dedupe
// semantics as the old per-row path: ON CONFLICT (chain, tx_hash, log_index)
// DO NOTHING. The ::numeric casts are exact for decimal text; an out-of-range
// value (e.g. > NUMERIC(78,0)) errors here and rolls the batch back.
const insertFromStage = `
INSERT INTO payments (
    chain, tx_hash, log_index,
    block_number, block_timestamp,
    source, protocol,
    facilitator, payer, payee, payee_service_id,
    asset, token_address, amount_raw, amount_usdc, asset_usd_at_time,
    auth_nonce,
    method_selector, called_contract, tx_type, tx_nonce,
    gas_used, effective_gas_price, gas_cost_wei, base_fee_per_gas,
    max_fee_per_gas, max_priority_fee_per_gas
)
SELECT
    chain, tx_hash, log_index,
    block_number, block_timestamp,
    source, protocol,
    facilitator, payer, payee, payee_service_id,
    asset, token_address,
    amount_raw::numeric, amount_usdc::numeric, asset_usd_at_time::numeric,
    auth_nonce,
    method_selector, called_contract, tx_type, tx_nonce,
    gas_used, effective_gas_price::numeric, gas_cost_wei::numeric, base_fee_per_gas::numeric,
    max_fee_per_gas::numeric, max_priority_fee_per_gas::numeric
FROM ` + stageTable + `
ON CONFLICT (chain, tx_hash, log_index) DO NOTHING`

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

	if len(batch) > 0 {
		if err := copyBatch(ctx, tx, batch); err != nil {
			return err
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

// copyBatch bulk-loads batch into a fresh staging table via pgx.CopyFrom, then
// moves the rows into payments with the deduping INSERT … SELECT. Both run on
// the caller's transaction, so the load and the cursor advance commit (or roll
// back) together — same atomicity as the row-by-row path it replaces.
func copyBatch(ctx context.Context, tx pgx.Tx, batch []x402.Payment) error {
	if _, err := tx.Exec(ctx, createStageTable); err != nil {
		return fmt.Errorf("create stage table: %w", err)
	}

	rows := make([][]any, len(batch))
	for i := range batch {
		rows[i] = copyRow(&batch[i])
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{stageTable}, copyColumns, pgx.CopyFromRows(rows)); err != nil {
		return fmt.Errorf("copy into stage: %w", err)
	}

	if _, err := tx.Exec(ctx, insertFromStage); err != nil {
		return fmt.Errorf("insert from stage: %w", err)
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

// copyRow flattens a Payment into the staging table's column order (see
// copyColumns). The six NUMERIC columns are emitted as text so COPY's binary
// encoder never has to encode a shopspring decimal; Postgres casts them back to
// NUMERIC on the INSERT … SELECT. base_fee_per_gas stays nil (→ SQL NULL) when
// the source tx carried no base fee, preserving the nullable column.
func copyRow(p *x402.Payment) []any {
	// Nullable big.Int columns: emit nil (→ SQL NULL) when the source tx/block
	// carried no value, preserving the nullable columns.
	nullableWei := func(v *big.Int) any {
		if v == nil {
			return nil
		}
		return v.String()
	}

	return []any{
		p.Chain, p.TxHash, int32(p.LogIndex), //nolint:gosec // log_index fits in int32; receipts cap well below 2^31
		int64(p.BlockNumber), p.BlockTimestamp, //nolint:gosec // Base block numbers will never approach 2^63
		p.Source, p.Protocol,
		p.Facilitator, p.Payer, p.Payee, p.PayeeServiceID,
		p.Asset, p.TokenAddress, p.AmountRaw.String(), p.AmountUSDC.String(), p.AssetUSDAtTime.String(),
		p.AuthNonce,
		p.MethodSelector, p.CalledContract, int16(p.TxType), int64(p.TxNonce), //nolint:gosec // tx_nonce fits in int64 for centuries
		int64(p.GasUsed), p.EffectiveGasPrice.String(), p.GasCostWei.String(), nullableWei(p.BaseFeePerGas), //nolint:gosec // gas_used realistic blocks << 2^63
		nullableWei(p.MaxFeePerGas), nullableWei(p.MaxPriorityFeePerGas),
	}
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
