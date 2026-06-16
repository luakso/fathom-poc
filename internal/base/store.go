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
	"time"

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
//
// LOCKSTEP: copyColumns, createStageTable, insertFromStage (its INSERT list AND
// its SELECT list), and copyRow must all list the same columns in the same
// order. Adding or removing a column means touching all four — a mismatch
// silently writes values into the wrong columns.
var copyColumns = []string{
	"chain", "tx_hash", "log_index",
	"block_number", "block_timestamp",
	"source", "protocol",
	"facilitator", "payer", "payee", "payee_service_id",
	"asset", "token_address", "amount_raw", "asset_usd_at_time",
	"auth_nonce",
	"method_selector", "called_contract", "tx_type", "tx_nonce",
	"gas_used", "effective_gas_price", "gas_cost_wei", "base_fee_per_gas",
	"max_fee_per_gas", "max_priority_fee_per_gas",
	"settlement_kind", "self_settled", "valid_after", "valid_before",
	"input_calldata", "block_hash", "transaction_index",
	"token_decimals", "token_symbol",
	"l1_fee", "l1_gas_used", "l1_gas_price", "tx_value", "gas_limit",
}

// createStageTable types the NUMERIC columns (amount_raw, asset_usd_at_time,
// effective_gas_price, gas_cost_wei, base_fee_per_gas, max_fee_per_gas,
// max_priority_fee_per_gas, l1_fee, l1_gas_used, l1_gas_price, tx_value) as TEXT.
// pgx's COPY uses the binary wire format and
// there is no shopspring decimal codec registered on the pool, so decimals
// cannot be binary-encoded into NUMERIC directly. Staging them as TEXT and
// casting ::numeric on the INSERT … SELECT keeps full precision without a codec
// dependency. amount_usdc is absent: it is a GENERATED column derived from
// amount_raw (see migration 00008) and must not be written.
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
    max_priority_fee_per_gas text,
    settlement_kind     text,
    self_settled        boolean,
    valid_after         timestamptz,
    valid_before        timestamptz,
    input_calldata      bytea,
    block_hash          text,
    transaction_index   integer,
    token_decimals      smallint,
    token_symbol        text,
    l1_fee              text,
    l1_gas_used         text,
    l1_gas_price        text,
    tx_value            text,
    gas_limit           bigint
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
    asset, token_address, amount_raw, asset_usd_at_time,
    auth_nonce,
    method_selector, called_contract, tx_type, tx_nonce,
    gas_used, effective_gas_price, gas_cost_wei, base_fee_per_gas,
    max_fee_per_gas, max_priority_fee_per_gas,
    settlement_kind, self_settled, valid_after, valid_before,
    input_calldata, block_hash, transaction_index, token_decimals, token_symbol
    , l1_fee, l1_gas_used, l1_gas_price, tx_value, gas_limit
)
SELECT
    chain, tx_hash, log_index,
    block_number, block_timestamp,
    source, protocol,
    facilitator, payer, payee, payee_service_id,
    asset, token_address,
    amount_raw::numeric, asset_usd_at_time::numeric,
    auth_nonce,
    method_selector, called_contract, tx_type, tx_nonce,
    gas_used, effective_gas_price::numeric, gas_cost_wei::numeric, base_fee_per_gas::numeric,
    max_fee_per_gas::numeric, max_priority_fee_per_gas::numeric,
    settlement_kind, self_settled, valid_after, valid_before,
    input_calldata, block_hash, transaction_index, token_decimals, token_symbol
    , l1_fee::numeric, l1_gas_used::numeric, l1_gas_price::numeric, tx_value::numeric, gas_limit
FROM ` + stageTable + `
ON CONFLICT (chain, tx_hash, log_index) DO NOTHING`

// insertCancellation persists one AuthorizationCanceled event. Cancellations are
// rare (an abandonment signal), so a per-row Exec inside the batch transaction is
// simpler than the COPY→stage machinery payments needs. Idempotent via the PK.
const insertCancellation = `
INSERT INTO authorization_cancellations
    (chain, tx_hash, log_index, authorizer, nonce, block_number, block_time, transaction_from)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (chain, tx_hash, log_index) DO NOTHING`

// insertCancellations writes every cancellation on the caller's transaction, so
// they commit (or roll back) together with the payments batch and the cursor
// advance — same atomicity guarantee as payments.
func insertCancellations(ctx context.Context, tx pgx.Tx, cancellations []x402.Cancellation) error {
	for i := range cancellations {
		c := &cancellations[i]
		if _, err := tx.Exec(
			ctx, insertCancellation,
			c.Chain, c.TxHash, int32(c.LogIndex), //nolint:gosec // log_index fits int32
			c.Authorizer, c.Nonce,
			int64(c.BlockNumber), //nolint:gosec // block_number fits int64 for ages
			c.BlockTime, c.TransactionFrom,
		); err != nil {
			return fmt.Errorf("insert cancellation %s/%d: %w", c.TxHash, c.LogIndex, err)
		}
	}
	return nil
}

// Store wraps a pgxpool.Pool with the operations base-collector needs.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a Store. The pool's lifetime is owned by the caller.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// AssertSchema verifies the schema contract InsertBatch depends on:
// payments.amount_usdc must be a GENERATED column (migration 00008), because
// this binary's INSERT deliberately omits it. The check makes a mismatched
// deploy (new binary on a pre-00008 schema, or the reverse) fail fast at
// startup with a clear message instead of a cryptic per-batch insert error.
func (s *Store) AssertSchema(ctx context.Context) error {
	var generated string
	err := s.pool.QueryRow(
		ctx, `
		SELECT attgenerated::text FROM pg_attribute
		WHERE attrelid = 'payments'::regclass AND attname = 'amount_usdc' AND NOT attisdropped`,
	).Scan(&generated)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("schema check: payments.amount_usdc does not exist — apply migrations through 00008 before running this binary")
	}
	if err != nil {
		return fmt.Errorf("schema check: %w", err)
	}
	if generated != "s" {
		return errors.New("schema check: payments.amount_usdc is not a GENERATED column — this binary requires migration 00008 (64M-row rewrite; run it in a maintenance window) before it can insert")
	}
	return nil
}

// InsertBatch inserts every row in batch, writes every cancellation, and
// advances the cursor to maxBlock, all in one Postgres transaction. If any
// insert fails, the whole transaction rolls back — neither the rows, the
// cancellations, nor the cursor advance. Cancellations are written on the same
// transaction as the payments and cursor advance, so a crash can never advance
// the cursor past un-stored cancellations.
//
// Idempotent: ON CONFLICT (chain, tx_hash, log_index) DO NOTHING absorbs
// re-runs over the same range. The cursor advance is monotonic — a smaller
// maxBlock than the current cursor is a no-op.
//
// maxBlock == 0 skips the cursor advance entirely. This is the §7
// empty-batch guard: HyperSync can deliver a batch with no logs and
// max_block = 0, and blindly writing it would reset progress to genesis.
func (s *Store) InsertBatch(ctx context.Context, batch []x402.Payment, cancellations []x402.Cancellation, maxBlock uint64) error {
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

	if len(cancellations) > 0 {
		if err := insertCancellations(ctx, tx, cancellations); err != nil {
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
// copyColumns). The NUMERIC columns are emitted as text so COPY's binary
// encoder never has to encode a shopspring decimal; Postgres casts them back to
// NUMERIC on the INSERT … SELECT. The nullable big.Int columns
// (base_fee_per_gas, max_fee_per_gas, max_priority_fee_per_gas, l1_fee,
// l1_gas_used, l1_gas_price, tx_value) stay nil
// (→ SQL NULL) when the source carried no value. amount_usdc is intentionally
// omitted — it is a GENERATED column (migration 00008) the DB derives from
// amount_raw.
func copyRow(p *x402.Payment) []any {
	// Nullable big.Int columns: emit nil (→ SQL NULL) when the source tx/block
	// carried no value, preserving the nullable columns.
	nullableWei := func(v *big.Int) any {
		if v == nil {
			return nil
		}
		return v.String()
	}

	// Nullable *time.Time columns: same nil → SQL NULL pattern as nullableWei.
	nullableTime := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return *t
	}

	return []any{
		p.Chain, p.TxHash, int32(p.LogIndex), //nolint:gosec // log_index fits in int32; receipts cap well below 2^31
		int64(p.BlockNumber), p.BlockTimestamp, //nolint:gosec // Base block numbers will never approach 2^63
		p.Source, p.Protocol,
		p.Facilitator, p.Payer, p.Payee, p.PayeeServiceID,
		p.Asset, p.TokenAddress, p.AmountRaw.String(), p.AssetUSDAtTime.String(),
		p.AuthNonce,
		p.MethodSelector, p.CalledContract, int16(p.TxType), int64(p.TxNonce), //nolint:gosec // tx_nonce fits in int64 for centuries
		int64(p.GasUsed), p.EffectiveGasPrice.String(), p.GasCostWei.String(), nullableWei(p.BaseFeePerGas), //nolint:gosec // gas_used realistic blocks << 2^63
		nullableWei(p.MaxFeePerGas), nullableWei(p.MaxPriorityFeePerGas),
		p.SettlementKind, p.SelfSettled, nullableTime(p.ValidAfter), nullableTime(p.ValidBefore),
		p.InputCalldata, p.BlockHash, int32(p.TransactionIndex), //nolint:gosec // tx index is bounded by block tx count; far below 2^31
		int16(p.TokenDecimals), p.TokenSymbol,
		nullableWei(p.L1Fee), nullableWei(p.L1GasUsed), nullableWei(p.L1GasPrice),
		nullableWei(p.TxValue), int64(p.GasLimit), //nolint:gosec // gas limit << 2^63
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
