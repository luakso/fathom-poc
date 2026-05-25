# x402 Indexing on Base — Findings & Gotchas

A field guide from building an indexer for x402 protocol transactions on Base (Rust + SQLite). Self-contained, language-agnostic. Pseudocode only — the point is the concepts and the traps, not the implementation.

Audience: you, six months from now, starting a fresh x402 indexer in another language.

---

## Status & verification note

This doc was originally drafted from working code and project notes, then iterated through two review passes that checked selectors, signatures, primary sources, and external coverage claims. Where a claim is **verified** (independent `keccak256`, EIP-3009 spec text, verified FiatTokenV2_2 source, or a primary-source link), it's stated plainly. Where a claim is **observed but unverified** — selector attributions we couldn't trace, ratios from our own data only — it's marked **⚠** inline. When you start the new project, treat any ⚠ item as a hypothesis to test against your own data, not as fact.

What's firm now:

- All four EIP-3009 selectors (`0xe3ee160e`, `0xcf092995`, `0xef55bec6`, `0x88b7ab63`) plus the `cancelAuthorization` pair (`0x5a049a70`, `0xb7b72899`) and the `AuthorizationCanceled` event topic are independently keccak'd and cross-checked against the verified `FiatTokenV2_2` source and the EIP-3009 spec text.
- USDC v2.2 implementation address behind the Base proxy.
- Coinbase Facilitator address set (10 on Base) from `facilitators.x402.watch`, the publicly-listed cdp.coinbase.com operator endpoint.
- Multicall3 address.

What's still flagged:

- The 97/3 outer-tx sighash split — observed in our own data but not cross-verified against `x402scan` or an independent HyperSync probe. (See Section 2 for the discussion, Section 13 for the verification path.)
- `0x93d9c747` attribution as "settleAndExecute / x402 SettlementRouter" — no primary source surfaced in either review pass; selector is empirical but the function name and provenance need on-chain confirmation.
- External-source numbers (Sherlock, Decentralised.Co aggregations) — directionally consistent with the under-capture hypothesis but not independently audited.

The most consequential revision since the original draft: `0xcf092995` is **not** `receiveWithAuthorization` (as the first draft claimed) and is **not** an SCW-wallet-only indicator (as the second pass over-narrowed). It is the FiatTokenV2_2 **bytes-signature overload of `transferWithAuthorization`** — the facilitator's preferred shape for both EIP-1271 contract-wallet signatures and packed-65-byte ECDSA from EOA payers. The selector alone classifies the *call shape*, not the *payer wallet type*. See Section 2.

A second consequential addition: the EIP-3009 path is not the only one x402 uses. The `upto` scheme is built on **Permit2** (different contract, different events), `batch-settlement` uses escrow + off-chain vouchers, and Coinbase's facilitator supports any ERC-20 via Permit2 — not just USDC/EURC via EIP-3009. An EIP-3009-only indexer is structurally blind to all of these. See Section 12.

---

## TL;DR

1. **Filter on topic AND a *multi-value* sighash allow-list, not a single selector.** Both `0xe3ee160e` (classic-sig) and `0xcf092995` (bytes-sig) are valid `transferWithAuthorization` overloads on USDC v2.2. The Coinbase TypeScript reference facilitator prefers the bytes overload for both EOA-packed-ECDSA and EIP-1271 SCW signatures. A `sighash == 0xe3ee160e`-only filter drops the call shape that is probably the majority of x402 traffic by tx count. See Section 2.
2. **The facilitator is `tx.from`, not in the event.** Every indexed row needs a log → transaction join. Doing this per-log (N+1) is what makes naïve indexers expensive; do it per-block. See Section 3.
3. **Two data paths, one cursor.** Historical backfill via a streaming archive (Envio HyperSync) with server-side sighash filtering; live tail via RPC (Alchemy) with client-side filtering. Both write to the same table behind the same cursor — no special handoff. See Section 5.
4. **Companion-Transfer pairing is non-obvious.** Multicall transactions emit multiple `Transfer` + `AuthorizationUsed` pairs. Pair by *highest `log_index` strictly less than the auth's `log_index`*, not by minimum absolute distance. See Section 4.
5. **The primary key is `(chain, tx_hash, log_index)`**, not `tx_hash`. Multicall again. This is also what makes `INSERT OR IGNORE` give you idempotent resume for free. See Section 7.
6. **Discover facilitators from data, don't curate them — but anchor against `facilitators.x402.watch`.** That community-maintained registry is the publicly-listed cdp.coinbase.com operator endpoint with the on-chain addresses Coinbase actually uses (10 on Base alone). Use it as a known-good sanity-check set. There is no on-chain facilitator registry. See Section 9.
7. **EIP-3009 is not the whole x402 surface.** The `upto` scheme uses Permit2; `batch-settlement` uses escrow; the Coinbase facilitator processes any ERC-20 via Permit2 (not just USDC/EURC via EIP-3009). An EIP-3009-only indexer has a structural coverage hole. See Section 12.

---

## 1. The domain in 60 seconds

x402 is an HTTP payment protocol on top of EIP-3009. Three actors:

- **Agent (payer):** signs an EIP-3009 authorization off-chain. Never submits a transaction themselves.
- **Facilitator (`tx.from`):** the entity that actually sends the transaction, pays gas, and submits the agent's signed authorization on their behalf.
- **Recipient:** the service receiving the USDC payment.

The on-chain settlement function is `transferWithAuthorization` on the USDC contract (selector `0xe3ee160e` — the canonical EIP-3009 variant with `v,r,s` parameters). When it succeeds, the USDC contract emits two events:

| Event | What it tells you |
|---|---|
| `Transfer(from, to, value)` | The actual payment: payer, recipient, amount in raw USDC (6 decimals) |
| `AuthorizationUsed(authorizer, nonce)` | The EIP-3009 authorization was consumed |

Critically, the outer transaction is almost never a direct call to `transferWithAuthorization`. It's usually a wrapper:

| Selector | Function | What it is |
|---|---|---|
| `0xe3ee160e` | `transferWithAuthorization(from,to,value,validAfter,validBefore,nonce,v,r,s)` | EIP-3009 canonical — classic-signature variant inherited from FiatTokenV2. |
| `0xcf092995` | `transferWithAuthorization(...,bytes)` | Bytes-signature overload added in USDC v2.2 (Nov 2023) for EIP-1271 support. **Same on-chain effect, same `AuthorizationUsed` event, also x402.** Used by the Coinbase TypeScript reference facilitator for both packed-65-byte ECDSA (EOA payers) and EIP-1271 (SCW payers). The selector classifies the *call shape*, not the *payer wallet type*. The original draft mis-identified this as `receiveWithAuthorization`; see Section 2 for the corrected framing. |
| `0xef55bec6` | `receiveWithAuthorization(...,v,r,s)` | Payee-pull flow, classic signature. Not x402 (no facilitator). Selector verified against EIP-3009 spec text. |
| `0x88b7ab63` | `receiveWithAuthorization(...,bytes)` | Payee-pull flow, bytes signature. Not x402. Selector verified against FiatTokenV2_2 source. |
| `0x5a049a70` | `cancelAuthorization(...,v,r,s)` | Authorizer cancels a not-yet-used nonce. Emits `AuthorizationCanceled`, not `AuthorizationUsed` — so it doesn't reach an `AuthorizationUsed`-topic-filtered indexer at all, but worth knowing about. |
| `0xb7b72899` | `cancelAuthorization(...,bytes)` | Same, bytes-sig variant. |
| `0x93d9c747` | ⚠ Unattributed outer-tx selector observed in our data | Originally labeled "settleAndExecute / x402 SettlementRouter" in this project's notes; **no primary source corroborates that name**. Not a FiatTokenV2_2 function. Not found in `coinbase/x402`, `x402-rs`, `thirdweb x402`, `second-state/x402-facilitator`, or `payai` repos. A plausible candidate is thirdweb's EIP-7702 facilitator dispatcher, but unconfirmed. Resolve by opening any tx where this appears on Basescan, reading `tx.to`, and matching the verified ABI. |
| `0x82ad56cb` | `aggregate3(...)` — Multicall3 at `0xcA11bde05977b3631167028862bE2a173976CA11` on Base | Wrapper, may batch multiple settlements. |

**Why we read events, not just calldata.** Regardless of which wrapper a facilitator uses, USDC emits the same two events from its internal `_transferWithAuthorization`. So in principle event-based indexing is wrapper-agnostic and survives the wrapper set growing over time.

**In practice, our current implementation has several distinct under-capture paths:**

1. **Single-value sighash filter drops the bytes-sig call shape.** Filtering on `sighash == 0xe3ee160e` excludes all `0xcf092995` calls — and that selector is the Coinbase TS facilitator's preferred shape regardless of payer wallet type. This is probably the biggest miss by tx count. (See Section 2.)
2. **Outer-tx sighash filter drops wrapper-routed payments.** Payments routed through `settleAndExecute`, `aggregate3`, or other wrappers emit the right events but are filtered out because the outer tx's sighash doesn't match any allowed value.
3. **EIP-3009-only filter misses Permit2-based payments entirely.** The x402 `upto` scheme is Permit2-based — different contract (`0x000000000022D473030F116dDEE9F6B43aC78BA3`), different events (`SignatureTransfer`), no `AuthorizationUsed` to filter on. Coinbase's facilitator also processes non-USDC/EURC ERC-20 payments via Permit2. (See Section 12.)
4. **EIP-3009-only filter misses batch-settlement entirely.** Escrow + off-chain vouchers, doesn't emit `AuthorizationUsed`.

A general fix for (1) and (2) is to AND the topic filter with a multi-value sighash allow-list (including `0xcf092995` and known wrappers), then exclude true `receiveWithAuthorization` (`0xef55bec6`, `0x88b7ab63`) post-hoc. Fixes for (3) and (4) require indexing entirely different events from different contracts. See Section 13 for the open work.

---

## 2. The filtering trap: `AuthorizationUsed` is ambiguous

The first instinct is: "subscribe to `AuthorizationUsed` on USDC and you're done." This is wrong — but the *reason* it's wrong is more nuanced than the original draft of this doc claimed. The framing here has been through two rounds of correction; what's below is what we believe holds up.

`AuthorizationUsed` is emitted by *every* EIP-3009 settle entry point on USDC v2.2:

- `transferWithAuthorization(...,v,r,s)` — selector `0xe3ee160e`
- `transferWithAuthorization(...,bytes)` — selector `0xcf092995`
- `receiveWithAuthorization(...,v,r,s)` — selector `0xef55bec6`
- `receiveWithAuthorization(...,bytes)` — selector `0x88b7ab63`

…plus any wrapper contract that calls one of those internally. The event by itself does **not** classify the call shape, the payer's wallet type, or whether it's x402.

### The two `transferWithAuthorization` overloads — and why the bytes variant matters

USDC v2.2 added the bytes-signature overload `transferWithAuthorization(...,bytes)` to support EIP-1271 (contract-wallet signatures). But the bytes overload is **also valid for EOA signers** — the caller just packs `r || s || v` into the bytes argument. The Coinbase TypeScript reference facilitator (the dominant x402 facilitator on Base) prefers the bytes overload across the board; the `x402-rs` facilitator historically preferred the classic `(v,r,s)` overload, which is why their issue [#26 "TransferWithAuthorization frequently reverts 'FiatTokenV2: invalid signature'"](https://github.com/coinbase/x402-rs) exists and explicitly explains the distinction:

> "For ERC-3009 TransferWithAuthorization, the facilitator should submit a plain 65-byte ECDSA (low-s) signature (EIP-712). If a signature is ERC-6492-wrapped, unwrap and pass the inner 65-byte ECDSA. If the signature is truly EIP-1271 (contract wallet), do not submit those bytes to ERC-3009's ECDSA path (FiatTokenV2 validates via ECDSA)."

**Implication:** the selector `0xcf092995` classifies the call shape (the facilitator chose the bytes overload), not the payer wallet type. The doc's earlier framing — "0xcf092995 means smart-contract-wallet payer" — was wrong. Determining EOA-vs-SCW for a specific payment requires `eth_getCode(authorization.from)` where `authorization.from` is decoded from calldata. `eth_getCode(tx.from)` is the wrong query (`tx.from` is the facilitator).

### What we observed empirically (⚠ unverified externally)

In our own data, captured by filtering on `AuthorizationUsed` topic + `sighash == 0xe3ee160e`, the breakdown of outer-tx sighashes we *would* see if we removed the sighash filter was reported as roughly:

| Outer-tx sighash | Share (our index) |
|---|---|
| `0xcf092995` | ~97% |
| `0xe3ee160e` | ~3% |

⚠ **This 97/3 split has not been independently reproduced** against `x402scan` or a fresh HyperSync probe with no sighash filter. It comes from our project's design notes. When you start the new indexer, the first probe to run is exactly this measurement, since it determines the cost-coverage trade-off of every downstream decision.

### What this means for the filter

Our current filter is:

```
WANT: AuthorizationUsed log
      AND parent_tx.input[0..4] == 0xe3ee160e
```

It captures the classic-signature `transferWithAuthorization` path cleanly. Everything else gets dropped — including the bytes-overload calls (`0xcf092995`), wrapper-routed calls (`settleAndExecute`, `aggregate3`, etc.), and entirely separate schemes (Permit2-based `upto`, batch-settlement). If the 97/3 split is even directionally correct, we are dropping the majority of x402 traffic by tx count.

### What the filter probably should be

```
WANT: AuthorizationUsed log
      AND parent_tx.input[0..4] IN {
        0xe3ee160e,       # transferWithAuthorization classic-sig
        0xcf092995,       # transferWithAuthorization bytes-sig
        ...known wrapper selectors discovered from data...
      }
      AND parent_tx.input[0..4] NOT IN {
        0xef55bec6,       # receiveWithAuthorization classic-sig (payee-pull)
        0x88b7ab63,       # receiveWithAuthorization bytes-sig (payee-pull)
      }
```

The right architecture is **"permissive sighash allow-list + explicit exclusion of payee-pull selectors + `AuthorizationUsed` presence as the final gate."** Each widening of the allow-list brings some genuine non-x402 back in; the explicit excludes catch the obvious payee-pull cases, and the topic check is the final filter. For wrappers, you may need a second pass that confirms the internal call hit USDC's `transferWithAuthorization` — but the `AuthorizationUsed` topic on USDC mostly does that for you.

### Cross-check against external sources (⚠ partial verification)

Multiple third-party numbers sit far outside what our indexer captured (~3.7M txs / ~$15.3M for Jan–Apr 2026 in Appendix B). The most reproducible primary source is the Dune dashboard `@hashed_official/x402-analytics`, which a Stacy Muur snapshot (Nov 28, 2025) summarized as **~40M total x402 tx over two months, 95% on Base, 98.6% USDC, ~$0.90 average payment on EVM**. The community-built block explorer `x402scan.com/facilitators` provides directly-queryable data with the same orientation. Less-reproducible aggregators (Sherlock's "119M Base / $35M" figure, Phemex citing Decentralised.Co at "18.82M total, +35×") are directionally consistent but don't cite primary sources.

Whichever number you anchor against, the gap relative to our capture is at least an order of magnitude by tx count. Our average payment size implied by our index (~$4) is also an order of magnitude above the ~$0.30–0.90 ecosystem average — consistent with "caught the big EOA-classic-sig payments, missed the long tail." Independent confirmation requires either an x402scan SQL query or a HyperSync probe with the widened sighash filter; both are listed in Section 13.

### Bottom line

Topic-alone filtering is too wide; topic + single-sighash filtering is too narrow. The first draft was right about the *trap* and wrong about the *fix* in two ways:

1. It assumed `0xcf092995` was `receiveWithAuthorization` (wrong — it's the bytes-sig `transferWithAuthorization`).
2. The first revision over-corrected by labeling `0xcf092995` as the "SCW-payer path" (also wrong — it's the facilitator's preferred shape regardless of payer wallet type).

In the new indexer, treat sighash as a multi-value allow-list, measure the actual sighash distribution before deciding the allow-list, and don't infer payer wallet type from the call shape.

---

## 3. The join trap: the facilitator isn't in the event

Naïve event-only indexing tells you the payer (`authorizer` from `AuthorizationUsed.topics[1]`) and amount/recipient (from the companion `Transfer`), but **not** the facilitator. The facilitator is `tx.from` — only available by joining with the parent transaction.

You can do this join two ways:

### N+1 (the trap)

```
logs = eth_getLogs(USDC, AuthorizationUsed topic, [from, to])
for each log:
    tx      = eth_getTransactionByHash(log.tx_hash)     # 1 RPC per log
    receipt = eth_getTransactionReceipt(log.tx_hash)    # 1 RPC per log
```

Two RPCs per log, every log including the ~97% that aren't your single chosen sighash. Hits free-tier quotas in minutes; expensive on paid tiers. Don't.

### Block-level batching (what worked)

```
logs = eth_getLogs(USDC, AuthorizationUsed topic, [from, to])
blocks_with_events = unique block numbers from logs

# 1 RPC per block (not per log), in parallel
for block in blocks_with_events:               # buffer_unordered(10)
    full_block = eth_getBlockByNumber(block, include_full_txs=true)

# Filter by sighash *before* fetching receipts.
# What we shipped: a single-value match (input[0..4] == 0xe3ee160e).
# What you should do: a multi-value allow-list per Section 2 — at
# minimum {0xe3ee160e, 0xcf092995}, excluding {0xef55bec6, 0x88b7ab63}.
x402_blocks = blocks where any tx in (logs of that block)
              has input[0..4] IN allowed_sighashes

# Only fetch receipts for blocks that survived the filter
for block in x402_blocks:                      # buffer_unordered(10)
    receipts = eth_getBlockReceipts(block)

# Now decode locally — no more RPCs
```

Key insights:

- One full-block fetch per block, not per log. Blocks with many x402 txs amortize beautifully.
- The sighash filter runs *between* the block fetch and the receipt fetch, so blocks whose txs all fall outside the allow-list pay nothing extra (no receipt fetch).
- Sub-batch concurrency (`buffer_unordered(10)` or your language's equivalent) gives near-linear speedup without overwhelming the RPC provider.
- `eth_getBlockReceipts` returns *all* receipts in a block. This is more bytes than per-tx fetching, but vastly fewer round-trips. On dense blocks this is a clear win; on sparse blocks it's a small overpay. Net win at our shape.

> ⚠️ **API gotcha:** Some RPC libraries default `getBlockByNumber` to returning transaction *hashes* only. You need full transaction objects (with `input`) for the sighash filter. Verify your library's default.

---

## 4. Decoding the payment: companion-Transfer pairing

`AuthorizationUsed` gives you the nonce and authorizer. The amount and recipient come from a separate `Transfer` event emitted in the same transaction by the same USDC contract. You have to pair them up — and this is subtler than it looks.

In a normal single-payment tx, USDC emits `Transfer` *immediately before* `AuthorizationUsed` from the same internal call. Easy:

```
[ ... Transfer(from=payer, to=recipient, value=X) ]   log_index = N
[ ... AuthorizationUsed(authorizer=payer, nonce)  ]   log_index = N+1
```

But multicall and SettlementRouter transactions bundle multiple payments into one tx:

```
[ Transfer(A→A', 1 USDC) ]    log_index = 0
[ AuthorizationUsed(A, nA) ]  log_index = 1
[ Transfer(B→B', 5 USDC) ]    log_index = 2
[ AuthorizationUsed(B, nB) ]  log_index = 3
```

The intuitive "find the closest `Transfer` to this `AuthorizationUsed`" strategy pairs `AuthorizationUsed(B)` with `Transfer(A→A')` (distance 3 vs distance 1) — silently corrupting the data.

**The correct rule:** for each `AuthorizationUsed` at log_index `K`, pair it with the **USDC `Transfer` log with the highest `log_index` strictly less than `K`**.

```
function pair_transfer(receipt_logs, auth_log_index):
    best = None
    for log in receipt_logs:
        if log.address != USDC: continue
        if log.log_index >= auth_log_index: continue   # must precede
        if best is None or log.log_index > best.log_index:
            best = log
    return decode_transfer(best)
```

Also: a `Transfer` *after* the `AuthorizationUsed` is never the right match — it belongs to a later payment in the same tx, or unrelated activity (e.g., the SettlementRouter forwarding a fee). Don't be tempted to fall through to it.

---

## 5. Two data paths: live tail vs historical archive

A single data path cannot do both jobs well. Use two.

### Historical backfill — streaming archive

For backfilling from chain genesis to recent tip, RPC providers are the wrong tool. ~3.5M blocks × per-batch round-trips × free-tier rate limits = days or weeks.

Use a chain-archive service that streams pre-indexed data with server-side filtering. We used Envio HyperSync. The model:

```
ONE query:
    logs.address          = USDC
    logs.topic0           = AuthorizationUsed signature hash
    transactions.sighash  IN allowed_sighashes  ← server-side filter; see Section 2
    fields                = {logs, txs, blocks} needed by the decoder
    block_range           = [genesis, tip - 500]

STREAM response batches:
    each batch is a (logs, transactions, blocks) bundle already joined
    build tx_by_hash and block_by_number maps from the batch
    for each log: join with its tx and block → enriched row
    decode → IndexedTx
    INSERT OR IGNORE  +  advance cursor to max(block in batch)
```

(What we shipped used a single-value `sighash = 0xe3ee160e`. That gave a ~30× bandwidth reduction but per Section 2 it also dropped the bytes-overload call shape — probably most of the x402 traffic by tx count. For the new project, use a multi-value allow-list.)

Properties:

- **One query, many response batches** — the server does the heavy lifting. We pay for one connection, not millions of RPC calls.
- **Server-side sighash filter** — the ~30× bandwidth reduction from Section 2. Cost-effective for the backfill, but per the revised reading of Section 2 it also drops bytes-signed x402 traffic. If you widen the sighash allow-list to include `0xcf092995`, the bandwidth saving shrinks and your stream gets larger; on a backfill that may still be fine, but probe it before committing.
- **Backfill stays further behind the tip than live sync** (500 blocks vs 6). Backfill is latency-insensitive and the worst-case reorg depth on a long-running stream is unbounded in practice; cheap insurance.

### Live tail — RPC

Once history is loaded, the tail is small (~tens of thousands of txs/day). RPC is fine. The block-level batching strategy from Section 3 keeps it within a free tier.

### The cursor is the contract between them

```
sync_state table: (chain TEXT PK, last_block INTEGER)
```

Both binaries read `sync_state` before starting and write it after each batch. The backfill stops at `tip - 500`; the live sync resumes from `cursor + 1` (or genesis if empty) and runs to `tip - 6`. **No special handoff code.** The shared cursor and idempotent inserts mean overlap is harmless.

### Probe before run

Both paths benefit from a dry-run mode. For HyperSync we built a `probe` subcommand that runs the exact same query but writes nothing — it just counts events, rows, decode failures, request count, bytes, elapsed time. Always run it before committing to a full backfill so you know the cost shape before you spend it.

---

## 6. Reorgs & cursors

### Confirmation depth — different per path

- **Live tail: 6 blocks behind tip.** Base has ~2s block time, so this is ~12 seconds of latency. Reorgs deeper than 6 blocks are rare on Base under normal operation, but bear in mind that Base's safety here comes from a centralized sequencer (operator policy), not protocol-level finality. The sequencer has had multi-hour outages and at least one restart-related larger-than-usual reorg has been reported historically. "6 blocks" is the right default for normal operation; treat the backstop as operational, not cryptographic.
- **Historical backfill: 500 blocks behind tip.** Backfill is not latency-sensitive; a long stream can race against reorgs at the leading edge. 500 is cheap insurance and the live sync will close the gap.

### Idempotent resume

```
PRIMARY KEY (chain, tx_hash, log_index)
INSERT OR IGNORE INTO transactions ...
```

The PK is what makes resume safe. After a crash, re-running re-emits some rows for the last committed block; the PK silently drops them. No dedup logic in the indexer.

Cursor is advanced **per batch, not per row** — one cursor write per response, after all rows in the batch have been inserted, all in one DB transaction. This gives crash-safe progress without per-row commit overhead.

### Cursor empty-batch guard

A subtle one: in the streaming backfill, an empty response can arrive with `max_block = 0`. If you blindly write the cursor, you reset progress to genesis. Guard:

```
if batch.max_block > 0:
    set_sync_cursor(chain, batch.max_block)
```

---

## 7. Storage: SQLite that survives concurrent access

SQLite is the right answer for a single-machine indexer with one writer and many readers (sync writes, CLI/analytics reads). But out-of-the-box defaults will bite you.

### PRAGMAs that matter

```sql
PRAGMA journal_mode = WAL;       -- concurrent readers + 1 writer
PRAGMA synchronous = NORMAL;     -- still crash-safe, ~10× faster commits than FULL
PRAGMA busy_timeout = 5000;      -- wait, don't immediately fail, when locked
PRAGMA temp_store = MEMORY;      -- keep GROUP BY scratch in RAM
```

WAL is the headline: without it, readers block writers and vice versa, and the analytics CLI ends up fighting the indexer for the database lock. With WAL, the indexer keeps writing while readers query consistent snapshots.

### Schema

The core table is straightforward. The instructive bits:

```sql
CREATE TABLE transactions (
    chain           TEXT    NOT NULL,
    tx_hash         TEXT    NOT NULL,
    log_index       INTEGER NOT NULL,        -- NOT redundant — multicall
    block_number    INTEGER NOT NULL,
    block_timestamp INTEGER NOT NULL,
    facilitator     TEXT    NOT NULL,        -- tx.from
    payer           TEXT    NOT NULL,        -- Transfer.from
    recipient       TEXT    NOT NULL,        -- Transfer.to
    amount_raw      TEXT    NOT NULL,        -- u256 as decimal string (full precision)
    amount_usdc     REAL    NOT NULL,        -- amount_raw / 10^6 (for queries)
    nonce           TEXT    NOT NULL,        -- the auth nonce (bytes32 hex)

    -- Captured for after-the-fact analysis; you'll want them eventually
    method_selector     TEXT,                -- first 4 bytes of calldata, hex
    called_contract     TEXT,                -- tx.to (router? multicall? direct?)
    tx_type             INTEGER,             -- 0=legacy 1=EIP-2930 2=EIP-1559
    gas_used            INTEGER,
    effective_gas_price TEXT,                -- u128 as decimal string
    gas_cost_eth        REAL,                -- gas_used * effective_gas_price / 1e18
    base_fee_per_gas    INTEGER,
    tx_nonce            INTEGER,             -- ⚠️ NOT the same as `nonce` above

    PRIMARY KEY (chain, tx_hash, log_index)
);

CREATE INDEX idx_tx_facilitator  ON transactions(facilitator);
CREATE INDEX idx_tx_payer        ON transactions(payer);
CREATE INDEX idx_tx_recipient    ON transactions(recipient);
CREATE INDEX idx_tx_chain_block  ON transactions(chain, block_number);
CREATE INDEX idx_tx_chain_ts     ON transactions(chain, block_timestamp);
```

Notes:

- **PK is `(chain, tx_hash, log_index)`**, not `tx_hash`. Multicalls put multiple authorizations in one tx, so `tx_hash` alone is not unique. This is also the substrate for `INSERT OR IGNORE` idempotency.
- **`amount_raw` is TEXT.** SQLite has no native u256, and casting through f64 quietly loses precision on large values. Store the canonical big-integer as a decimal string; derive the human-readable f64 alongside for query convenience. Never compute aggregates from f64 if precision matters.
- **Capture the routing metadata** (`method_selector`, `called_contract`, `tx_type`). It costs nothing during sync but lets you analyze wrapper churn, fee economics, and EIP-1559 vs legacy mix later without re-indexing.
- **The chain column is cheap insurance.** Single-chain today, multi-chain tomorrow, and it's the natural prefix on every index.

### Batch inserts in one transaction

The single biggest write performance lever in SQLite:

```
db_tx = conn.begin_transaction()
for row in batch:
    db_tx.execute("INSERT OR IGNORE INTO transactions VALUES (...)", row)
db_tx.commit()
```

Without the explicit transaction, every `INSERT` auto-commits with its own fsync. For a 1k-row batch this is 1000× the disk syncs. Wrapping in one transaction is two orders of magnitude faster and atomic across the batch.

### Idempotent column migrations

SQLite has no `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`. The pattern that worked for us:

```
existing_columns = query("PRAGMA table_info(transactions)")
for (name, type) in NEW_COLUMNS:
    if name not in existing_columns:
        execute("ALTER TABLE transactions ADD COLUMN {name} {type}")
```

This means migrations are declarative and re-runnable. We grew the schema from 14 → 22 columns over the project's life without ever needing a separate migration tool. For a single-binary CLI it's the right level of ceremony.

### Schema ownership boundary

If you have multiple processes (we eventually added a Python analytics layer alongside the Rust indexer), define an explicit boundary:

- **Ingestion tables** (`transactions`, `facilitators`, `sync_state`, `daily_stats`, `schema_version`) — written only by the indexer.
- **Analysis tables** (`wallet_scores`, `tx_scores`, etc.) — written only by the analytics layer.

Enforce the boundary in code. The non-owning process opens the DB read-only:

```
conn = sqlite_open(path, mode="ro")
conn.execute("PRAGMA query_only = ON")
```

And the test suite scans every migration file to assert it only targets tables it owns. Without this guarantee, the two processes race and migration ordering becomes a nightmare.

---

## 8. Domain-model pitfalls

These are the things that bit us repeatedly. Avoidable if you know about them.

### Two different "nonces"

```
nonce      = AuthorizationUsed nonce (bytes32) — the EIP-3009 authorization id
tx_nonce   = the Ethereum transaction sequence number of tx.from (uint64)
```

Different domain, different type, different size, easy to conflate in code review. Name them distinctly in your domain type. We use `nonce` and `tx_nonce` and document the difference at the field.

### Gas cost overflow

```
gas_used                : u64
effective_gas_price     : u128  (wei per gas unit)
gas_cost_wei = gas_used * effective_gas_price
```

If you multiply two `u64`s, you'll overflow on dense blocks. Widen `gas_used` to `u128` (or the equivalent in your language) before multiplying. Then `gas_cost_eth = wei / 1e18` as `f64` for storage. The `f64` loses precision but the displayed gas cost doesn't need 18 decimals.

### Streaming-archive integer encoding

HyperSync (and similar archives) encode numeric fields as canonical big-endian variable-length byte slices with leading zeros stripped. `0` is `[0x00]`, `1000` is `[0x03, 0xe8]`. To get a `u64`:

```
function quantity_to_u64(bytes):
    assert len(bytes) <= 8        # or use u128, or it'll silently truncate
    buf = [0; 8]
    buf[8 - len(bytes):] = bytes  # left-pad with zeros
    return be_bytes_to_u64(buf)
```

The trap: if a field ever exceeds the width you assumed (block timestamps becoming larger, gas_used on a giant block), the silent truncation gives you nonsense. Assert the length in debug builds, and pick `u128` for any field where you're not sure of the upper bound.

### Address case sensitivity

Ethereum addresses are case-insensitive (EIP-55 checksums use mixed case, but `0xAbC...` == `0xabc...` semantically). SQL `WHERE address = ?` is case-sensitive. Normalize to lowercase at the storage boundary, store lowercase, query with lowercase. Or wrap every query in `LOWER(...)` and pay the index cost. Pick one and stick to it.

---

## 9. Facilitator registry: discover, but anchor against `facilitators.x402.watch`

There is **no on-chain facilitator registry** for x402. There is no contract you can read to enumerate facilitators. The closest thing to canonical is the community-maintained directory at `facilitators.x402.watch`, which lists 19 facilitators across 94 addresses spanning Base / Solana / Polygon, each linked to the relevant block explorer.

That directory is your starting point. It is **not** the universe of facilitators (anyone can run one — that's the point of x402) but it gives you a known-good anchor set to validate your discovery query against. Specifically, the Coinbase entry lists 11 addresses (10 on Base, 1 on Solana) under the URL `https://facilitator.cdp.coinbase.com` — these are the on-chain addresses Coinbase actually uses for its hosted facilitator. The Coinbase Base addresses:

```
0xdbdf3d8ed80f84c35d01c6c9f9271761bad90ba6   ("Coinbase: x402 Facilitator 1")
0x9aae2b0d1b9dc55ac9bab9556f9a26cb64995fb9
0x3a70788150c7645a21b95b7062ab1784d3cc2104
0x708e57b6650a9a741ab39cae1969ea1d2d10eca1
0xce82eeec8e98e443ec34fda3c3e999cbe4cb6ac2
0x7f6d822467df2a85f792d4508c5722ade96be056
0x001ddabba5782ee48842318bd9ff4008647c8d9c
0x9c09faa49c4235a09677159ff14f17498ac48738
0xcbb10c30a9a72fae9232f41cbbd566a097b4e03a
0x9fb2714af0a84816f5c6322884f2907e33946b88
```

**Discovery story.** We started with a curated list of 5 "known" facilitator addresses baked into the binary, sourced from blog posts and Discord chatter. After our first real backfill, **none of them appeared in the data**. They were guesses. A one-line query gave us the truth:

```
SELECT facilitator, COUNT(*) AS txs, SUM(amount_usdc) AS volume
FROM transactions
GROUP BY facilitator
ORDER BY txs DESC;
```

500+ distinct `tx.from` addresses had settled x402 transactions in our indexed dataset. 22 of them matched our (corrected) curated metadata. The other ~480 are unlabeled.

**Caveats on the 500+ number:**

1. **Filter coverage.** Captured by the `sighash == 0xe3ee160e` filter. Per Section 2, that filter likely misses the majority of x402 traffic by tx count (the bytes-overload calls). Real facilitator population is almost certainly larger.
2. **"Facilitator" is loose terminology.** What we count as a "facilitator" is "tx.from address calling an EIP-3009 settle function on USDC with x402-shaped calldata." That's an operational definition, not a registry membership. Some of those addresses are EOAs run by named services; others are dispatcher contracts; some may be one-off automation. Treat the count as "candidates," not "facilitators."
3. **Multi-chain.** Coinbase's facilitator also runs on Solana, Polygon, Arbitrum, World per CDP docs. A Base-only index is single-chain by design but the *protocol* is multi-chain. Adjust your `chain` column accordingly.

**The lesson: presence in indexed data is the truth; the curated registry is metadata-only.** Treat the registry as a join table for human-friendly names, not as a filter. If you filter sync on the registry, you blind yourself to every new facilitator that launches.

The curated registry still has uses:
- Display names for top facilitators in the UI.
- Promoting a discovered address (after manual review) so it shows up as known.
- Seeding the `facilitators` table so day-one CLI output has names instead of just addresses.
- **Sanity-checking your sync.** If you don't see all 10 Coinbase Base addresses in your discovered list after a meaningful sync window, your filter is wrong before anything else can be.

We embed our registry as a TOML file compiled into the binary (`include_str!` equivalent — zero-deps bootstrap). The downside is that adding a name requires a rebuild; the upside is no missing-config errors on first run. For the new project, consider syncing `facilitators.x402.watch` into the registry table on startup, or at least linking to it from your CLI's `facilitators` command output.

---

## 10. Operational pattern catalogue

Small things that paid off.

### Clean shutdown between batches

```
shutdown_flag = AtomicBool::new(false)
spawn { ctrl_c().await; shutdown_flag.set(true) }

for batch in stream:
    process_and_commit(batch)
    if shutdown_flag.get(): break
```

Check the flag *after* committing, not in the middle. Ctrl-C between batches is clean; Ctrl-C mid-batch is an aborted DB transaction that rolls back cleanly. Either way the cursor reflects the last fully-committed batch and a re-run picks up perfectly. **Don't try to abort mid-batch** — you'll add bugs without saving meaningful time.

### Exponential backoff on rate limits

```
attempt = 0
loop:
    try:
        return rpc_call()
    catch err where is_rate_limit(err):
        attempt += 1
        if attempt > 5: re-raise
        sleep(min(5 ** attempt, 120) seconds)   # 5, 25, 120, 120, 120
    catch err:
        re-raise   # non-rate-limit errors fail fast
```

Only retry on 429 / "rate limit" / "too many requests". Fail fast on everything else — silent retry on a schema error wastes hours.

### Sub-batch concurrency

```
results = stream(blocks_with_events)
    .map(async block -> fetch_full_block(block))
    .buffer_unordered(10)            # or your language's parallel-flat-map equivalent
    .collect()
```

Concurrency factor 10 is the sweet spot for a free-tier Alchemy. Higher hits rate limits; lower leaves throughput on the table. Tune by watching the provider's dashboard.

### Capture the routing metadata at sync time

`method_selector`, `called_contract`, `tx_type`, `gas_used`, `effective_gas_price`, `base_fee_per_gas`, `tx_nonce` — store all of these as columns even if you don't use them on day one. They cost almost nothing at sync time (you already have them in hand from the parent transaction). Re-indexing to add them later costs hours and burns RPC budget. Future analysis (wrapper-share over time, gas/payment ratio, EIP-1559 adoption) needs them.

### Probe subcommand on the backfill

Already mentioned in Section 5, but worth its own line: every long-running data pull should have a dry-run mode that does the same query and counts. Knowing "this will be 3.7M events, 80MB, 22 minutes" before you commit is worth the hour to build it.

### Resolve DB path with explicit precedence

```
db_path = flag --db
       OR env $X402_DB_PATH
       OR ~/.x402/index.db   (with auto-mkdir)
```

Trivial, but cleans up dev/prod/test configuration. Auto-creating the parent directory on the default path means a fresh install Just Works without a setup step.

---

## 11. What I'd do differently

Honest reflections.

- **Start with HyperSync-style archive from day one.** We spent real time tuning the Alchemy path before realizing the historical load was the actual bottleneck. The Alchemy path matters only for the live tail, and that's a much smaller engineering problem.
- **Build the probe subcommand before the run subcommand.** Twice we kicked off long runs that we then had to abort because the data didn't look right. The probe is one screen of code and pays for itself the first time.
- **Use `(chain, tx_hash, log_index)` as the PK from day one, not after the first multicall bug report.** We learned this the hard way.
- **Treat the curated facilitator list as a UI label table, not a filter, from the start.** We filtered on it briefly and missed weeks of new facilitator launches.
- **Don't compute aggregates from `amount_usdc` (f64) for anything that matters.** Sum from `amount_raw` (string → big-int) and divide at the end. f64 sums are fine for dashboards, lethal for accounting.
- **Pick a clear schema-ownership boundary the day you add a second writer.** Adding it retroactively is a refactor; building it in is two functions and a test.
- **Capture `called_contract` and `method_selector` from day one.** We added them later in a column migration; doing so backfilled `NULL` for half the dataset, which made wrapper-share analysis annoying.
- **Don't try to migrate or normalize amounts at insert time.** Store raw + derived. Re-derivation is cheap; re-decoding is not.
- **Compute selectors instead of trusting prose attributions.** The original draft of this doc identified `0xcf092995` as `receiveWithAuthorization` because that's what our project's design notes said. Independent `keccak256` against the verified FiatTokenV2_2 source shows it's actually `transferWithAuthorization(...,bytes)`. One five-minute selector recomputation on day one would have caught this and probably re-shaped the entire filter strategy. Build the new indexer with a small unit test that asserts `keccak256(SIGNATURE)[0:4] == NAMED_CONSTANT` for every selector you reference, so names can't drift from bytes.
- **Don't infer payer wallet type from the call selector.** The first revision of this doc over-corrected and labeled `0xcf092995` as "the SCW-payer path." It's not — the bytes overload is the Coinbase TS facilitator's preferred shape for both EOA-packed-ECDSA and EIP-1271 signatures. EOA-vs-SCW is offchain signature classification or `eth_getCode(authorization.from)`, not the selector. (And `eth_getCode(tx.from)` is the wrong query — that's the facilitator, not the payer.)
- **Anchor against a known-good facilitator set from day one.** `facilitators.x402.watch` lists 19 facilitators across 94 addresses, including the 10 Coinbase Base addresses (under the URL `https://facilitator.cdp.coinbase.com`) that Coinbase actually uses. If your "discovered facilitators" list doesn't include all 10 Coinbase addresses, your filter is wrong.
- **Cross-check your indexed totals against `x402scan.com` and Dune `@hashed_official` before declaring coverage complete.** Third-party numbers exceed our capture by an order of magnitude. We never ran that comparison while building; if we had, the coverage gaps would have surfaced in week one instead of post-mortem.
- **Treat EIP-3009 as one scheme out of several.** The `upto` scheme is Permit2; `batch-settlement` is escrow; non-USDC/EURC tokens via Coinbase's facilitator go through Permit2. An EIP-3009-only indexer is structurally incomplete from day one, not just under-tuned.

---

## 12. Coverage gaps beyond EIP-3009 — schemes the current indexer is blind to

The current indexer treats x402 as "EIP-3009 on USDC." That's a simplification that was true enough at the start of the project but is increasingly wrong. The x402 protocol defines (or has shipped) at least three schemes, only one of which goes through EIP-3009.

### `exact` — what we index

EIP-3009 on USDC (and EURC). Single fixed payment per authorization. Verified from the spec at `coinbase/x402/specs/schemes/exact/`. This is what every section of this doc up to here is about.

### `upto` — Permit2-based, completely separate event surface

The `upto` scheme is shipped on EVM (TS/Go/Python SDKs) per `coinbase/x402/specs/schemes/upto/scheme_upto.md`. It uses **Uniswap's Permit2** at the canonical address `0x000000000022D473030F116dDEE9F6B43aC78BA3`, specifically `permitWitnessTransferFrom` with the witness pattern enforcing the recipient. The x402 spec names a wrapper called **`x402ExactPermit2Proxy`** for this path (audited but no deployed address listed in the spec at time of writing).

**Implications for an indexer:**

- **No `AuthorizationUsed` event.** Permit2 emits `SignatureTransfer` events on the Permit2 contract itself. A USDC-and-`AuthorizationUsed`-filtered indexer captures **zero** `upto` traffic.
- **Different contract address.** Permit2 is `0x000000000022D473030F116dDEE9F6B43aC78BA3` on every EVM chain, not the USDC contract.
- **Different decode path.** `permitWitnessTransferFrom` has its own ABI; the recipient is enforced by the witness, not by the call args directly.
- **Token-agnostic.** Permit2 works with any ERC-20, not just USDC/EURC. That's the protocol point.

If your new indexer is going to cover `upto`, you need a second data path that filters on `address = Permit2 + topic = SignatureTransfer`. The downstream model (facilitator = `tx.from`, payer/recipient/amount from event data) is similar but the join shape differs.

### `batch-settlement` — escrow plus off-chain vouchers

Listed in the `x402-foundation/x402` repo README alongside `exact` and `upto`. Uses an escrow contract + off-chain redemption vouchers, then redeems on-chain in batches. **Does not emit USDC `AuthorizationUsed`** — entirely separate event surface, probably custom escrow-contract events.

We did not index this. We have no measurement of its share of x402 traffic. Worth probing.

### Coinbase facilitator processes any ERC-20 via Permit2

Per CDP docs (`docs.cdp.coinbase.com/x402/welcome`): *"CDP's x402 facilitator processes ERC-20 payments on Base, Polygon, Arbitrum, World, and Solana — via EIP-3009 (USDC, EURC) or Permit2 (any ERC-20)."*

So even on the `exact` scheme, **payments in any token that isn't USDC or EURC go through Permit2 instead of EIP-3009**. A USDC-only indexer is single-token by construction.

### EIP-7702 surface — open question

Thirdweb's facilitator explicitly uses EIP-7702 (`portal.thirdweb.com/x402/facilitator`: *"It uses your own server wallet and leverages EIP-7702 to submit transactions gaslessly"*). A 7702-delegated transaction is still an EOA-sent tx that temporarily acts as a smart contract during execution. The inner call to USDC's `transferWithAuthorization` still emits `AuthorizationUsed`, so the topic filter would catch the event — but the **outer tx's sighash** would reflect whatever the 7702 delegation contract dispatches with, not `0xe3ee160e`.

**Plausible-but-unconfirmed hypothesis:** the `0x93d9c747` selector observed in our data (and unattributed by either review pass) could be a thirdweb-EIP-7702 dispatcher selector. Worth probing: pick a tx where `0x93d9c747` appears as `tx.input[0..4]`, check `tx.to` on Basescan, see if it's a thirdweb-related contract. Don't filter by transaction `type` to detect 7702 — the indexer should be type-agnostic; `AuthorizationUsed` is the right gate.

### Summary table

| x402 scheme | Status | On-chain event | Indexer covers? |
|---|---|---|---|
| `exact` (EIP-3009 / USDC) | Shipped | `AuthorizationUsed` on USDC | ✅ Partial — see Section 2 for the sighash gap |
| `exact` (EIP-3009 / EURC) | Shipped | `AuthorizationUsed` on EURC | ❌ Different token, not indexed |
| `exact` (Permit2 / any ERC-20) | Shipped (via Coinbase facilitator) | `SignatureTransfer` on Permit2 | ❌ Different event surface, not indexed |
| `upto` (Permit2-witness) | Shipped | `SignatureTransfer` on Permit2 (witness-enforced recipient) | ❌ Different event surface, not indexed |
| `batch-settlement` (escrow + vouchers) | Specified | Custom escrow events | ❌ Different event surface, not indexed |
| Cloudflare's "deferred payment" proposal | Proposed, not finalized | TBD | ❌ |

**For the new project:** decide deliberately which schemes you're targeting before you start. EIP-3009-on-USDC alone covers (probably) the majority of *current* tx count but is a steadily shrinking share of the *protocol* as `upto`, Permit2-based non-USDC, and EIP-7702 paths grow. Build the indexer's data-source abstraction so that adding a Permit2 path later is "another stream into the same `transactions` table" rather than "rip and replace."

---

## 13. What's still open / future research

Listed roughly in priority order for the new project.

### Verify and act on the bytes-sig coverage gap (highest priority)

Per Section 2, our `sighash == 0xe3ee160e`-only filter likely drops the majority of x402 traffic by tx count. Two concrete ways to verify before re-shaping the filter:

1. **Query `x402scan.com`.** Merit Systems (also behind `awesome-x402`) built `x402scan` as a public, x402-specific block explorer with a SQL-style query API. A single query — "group `AuthorizationUsed`-emitting txs on Base USDC by outer-tx sighash, last 7 days, by count and volume" — resolves the entire question. This is the right anchor because it's purpose-built for x402.
2. **Run a HyperSync probe.** You already have the probe pattern from Section 5. One pass with `topic = AuthorizationUsed`, no sighash filter, group-by parent-tx sighash over ~50K blocks. Cheap, reproducible, gives you the actual empirical distribution.

If either confirms the 97/3 (or any large skew), widen the allow-list to `{0xe3ee160e, 0xcf092995}`, exclude `{0xef55bec6, 0x88b7ab63}`, re-ingest a sample range, and confirm captured row counts move toward the third-party totals.

### Cover Permit2-based schemes (`upto` + non-USDC ERC-20)

Per Section 12. Different contract (`0x000000000022D473030F116dDEE9F6B43aC78BA3`), different event (`SignatureTransfer`). Add as a second data path; the same `transactions` table schema mostly works if you make `token_address` a column (currently implied to be USDC) and treat `facilitator = tx.from` consistently. The Coinbase facilitator processes any ERC-20 via Permit2, so this isn't a "future protocols" thing — it's already production traffic you're missing.

### Cover `batch-settlement`

Spec'd but we have no measurement of its share. Probably a smaller bucket but should be sized before deciding to skip.

### Wrapper-routed payment capture

Independent from the bytes-sig issue. Filter on `sighash IN (...wrapper selectors discovered from data...)` plus topic. Trade-off: each new sighash you allow lets some non-x402 back in, so you need the `AuthorizationUsed`-present final gate.

### Verify `0x93d9c747` provenance on-chain

Neither review pass found a primary source. Plausible candidates worth probing:

- **Thirdweb's EIP-7702 facilitator dispatcher.** Their docs say they use 7702 for gasless submission; the outer-tx selector would reflect the delegation contract, not `transferWithAuthorization`. Pick any tx where `0x93d9c747` appears, check `tx.to` on Basescan, see if it resolves to a thirdweb-labeled contract.
- **The `x402ExactPermit2Proxy`** mentioned in the x402 spec (but no deployed address listed in the spec).
- A CDP-internal settlement wrapper.

If you confirm it, update Appendix A. If you can't, keep the ⚠ unattributed flag.

### `AuthorizationCanceled` handling

A nonce can move to a `cancelled` terminal state (via `cancelAuthorization`, selectors `0x5a049a70` / `0xb7b72899`, event `AuthorizationCanceled` at topic `0x1cdd46ff242716cdaa72d159d339a485b3438398348d68f09d7c8c0a59353d81`). Currently invisible to an `AuthorizationUsed`-only topic filter. Volume contribution probably small but should be measured. If you model nonces as a state machine, "cancelled" is its own terminal state, not equivalent to "used."

### EIP-7702 surface

Don't filter by transaction `type`. A 7702-delegated EOA settlement still emits `AuthorizationUsed` from USDC; the topic filter catches it. The outer-tx sighash is what changes (see `0x93d9c747` above as a candidate).

### Selector self-test

A small unit test that asserts `keccak256(SIGNATURE)[0:4] == NAMED_CONSTANT` for every named selector your indexer references. Catches drift between prose and bytes. Should have existed from day one.

### Anchor against `x402scan.com` + Dune `@hashed_official` in CI

Have your indexer's `status` command print captured totals alongside the live numbers from x402scan (or compare in a periodic job). Any large gap is an alert that a coverage path has opened up. We never did this; if we had, the bytes-sig gap would have surfaced in week one.

### Smaller items

- **Cross-chain identity.** Same wallet address on Base and Ethereum / Polygon / Solana is often (not always) the same entity. We didn't resolve this. The `chain` column makes it possible later.
- **Reorg detection.** We use a confirmation buffer (6 blocks live, 500 backfill) instead of explicit reorg detection. Cheap and correct for this use case. A production indexer that wants lower latency needs to track block hashes and roll back on mismatch.
- **Live sync via HyperSync.** We kept Alchemy for the tail because it was already working when we built the backfill. Unifying on HyperSync would simplify the code; we just never needed to.
- **Funding-source tracing.** A wallet's first inbound tx tells you a lot about whether it's organic. We didn't index the broader USDC `Transfer` stream that would let us do this cheaply.
- **Facilitator labeling at scale.** Hundreds of discovered facilitators, a small handful with human-friendly names. An LLM pass over wallet behavior + on-chain context could label them at scale.
- **Watch the USDC proxy for `Upgraded(address)` events.** Implementation behind the Base USDC proxy (as of May 2026) is `0x2Ce6311ddAE708829bc0784C967b7d77D19FD779` (FiatTokenV2_2); previous was `0x6d0c9a70d85e42ba8b76dc06620d4e988ec8d0c1`. Future Circle upgrades could change the ABI surface; watching `Upgraded(address)` on the proxy lets you alert before your indexer silently breaks.

---

## Appendix A — Key constants & signatures

Selectors below are verified by three independent paths: (a) the EIP-3009 spec text at `eips.ethereum.org/EIPS/eip-3009`, which embeds the literal `0xef55bec6`; (b) the verified `FiatTokenV2_2` source at `circlefin/stablecoin-evm`; (c) independent `keccak256` computation cross-checked across multiple review passes. Where a row is unconfirmed, it's marked **⚠**.

### Addresses (Base)

| Thing | Value |
|---|---|
| USDC proxy contract | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| USDC implementation (as of May 2026) | `0x2Ce6311ddAE708829bc0784C967b7d77D19FD779` (FiatTokenV2_2). Previous: `0x6d0c9a70d85e42ba8b76dc06620d4e988ec8d0c1`. Watch `Upgraded(address)` on the proxy to detect future Circle upgrades. |
| EURC contract (Base) | Different address, different deployment; if you're going to cover EURC, look it up separately and add a `token_address` column to your schema. |
| Permit2 contract (every EVM chain) | `0x000000000022D473030F116dDEE9F6B43aC78BA3` |
| Multicall3 contract | `0xcA11bde05977b3631167028862bE2a173976CA11` |
| Base chain ID | `8453` |
| Base "2026 genesis" (our chosen start block) | `40_222_720` (~Jan 1, 2026 02:33 UTC) |

### Event topic hashes (keccak256 of signature)

| Event | topic0 |
|---|---|
| `AuthorizationUsed(address,bytes32)` | `0x98de503528ee59b575ef0c0a2576a82497bfc029a5685b209e9ec333479b10a5` |
| `AuthorizationCanceled(address,bytes32)` | `0x1cdd46ff242716cdaa72d159d339a485b3438398348d68f09d7c8c0a59353d81` |
| `Transfer(address,address,uint256)` | `0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef` |
| `SignatureTransfer(...)` on Permit2 | Look up against the Permit2 ABI when you add the Permit2 path |

### Method selectors on FiatTokenV2_2 (first 4 bytes of keccak256(signature))

| Selector | Function | Is x402? |
|---|---|---|
| `0xe3ee160e` | `transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)` (classic-sig) | **Yes — canonical x402 settlement, classic signature** |
| `0xcf092995` | `transferWithAuthorization(address,address,uint256,uint256,uint256,bytes32,bytes)` (bytes-sig overload, USDC v2.2) | **Yes — also x402.** Facilitator's preferred call shape on the Coinbase TS reference implementation. Selector classifies *call shape*, not payer wallet type. |
| `0xef55bec6` | `receiveWithAuthorization(address,address,uint256,uint256,uint256,bytes32,uint8,bytes32,bytes32)` | **No — payee-pull flow, no facilitator.** Spec-anchored. |
| `0x88b7ab63` | `receiveWithAuthorization(address,address,uint256,uint256,uint256,bytes32,bytes)` | **No — payee-pull flow, bytes sig.** |
| `0x5a049a70` | `cancelAuthorization(address,bytes32,uint8,bytes32,bytes32)` | **N/A — emits `AuthorizationCanceled`, not `AuthorizationUsed`**. State transition: nonce → cancelled. |
| `0xb7b72899` | `cancelAuthorization(address,bytes32,bytes)` | Same. Bytes-sig variant. |

### Other observed selectors

| Selector | Function | Notes |
|---|---|---|
| `0x82ad56cb` | `aggregate3(...)` on Multicall3 | Wrapper, may batch multiple settlements. Verified. |
| `0x93d9c747` | **⚠ Unattributed.** Observed in our indexed data; originally labeled "settleAndExecute / x402 SettlementRouter" in project notes; no primary source corroborates. Not a FiatTokenV2_2 function. Not in `coinbase/x402`, `x402-rs`, `thirdweb x402`, `second-state/x402-facilitator`, or `payai` repos. Candidate hypothesis (unproven): thirdweb's EIP-7702 facilitator dispatcher. Resolve by opening any tx on Basescan and reading `tx.to`'s verified ABI. |

> **Filter guidance** (see Section 2 for full rationale): the original "keep `0xe3ee160e`, drop everything else" strategy under-captures the bytes-overload call shape. The right shape is closer to:
> ```
> topic == AuthorizationUsed
>   AND parent_tx.sighash IN { 0xe3ee160e, 0xcf092995, ...wrappers... }
>   AND parent_tx.sighash NOT IN { 0xef55bec6, 0x88b7ab63 }
> ```
> Capture `method_selector` and `called_contract` as columns regardless, so you can refine the filter after the fact without re-indexing. And remember EIP-3009 is one scheme out of several — see Section 12.

### Engineering constants we settled on

| Constant | Value | Why |
|---|---|---|
| `CONFIRMATION_DEPTH` (live sync) | 6 blocks | ~12s latency on Base, well clear of observed reorg depth |
| `REORG_SAFETY_BUFFER` (backfill) | 500 blocks | Long-running stream + reorg races; backfill is latency-insensitive |
| `BATCH_SIZE` (live sync) | 100 blocks per `eth_getLogs` | Default; tunable. Free tier handles this without throttling |
| `SYNC_THROTTLE_MS` | 1000 ms between batches | Free-tier-safe; reduce on paid tiers |
| Sub-batch concurrency | `buffer_unordered(10)` | Sweet spot on free-tier Alchemy |
| Retry attempts on rate limit | 5 | Backoff: 5s, 25s, 120s, 120s, 120s |
| USDC decimals | 6 | `1 USDC = 1_000_000 raw` |

---

## Appendix B — Data shape we observed (Apr 2026 snapshot)

For sanity checks against your own indexer. Numbers from our ~3.7M indexed txs, blocks `40_222_720` → live tip.

> **⚠ Coverage caveat — read this before treating any number below as authoritative.** These numbers reflect only what our `sighash == 0xe3ee160e` filter captured on USDC. Per Sections 2 and 12, that filter misses (at minimum): the bytes-overload call shape `0xcf092995`, wrapper-routed payments, the Permit2-based `upto` scheme, the escrow-based `batch-settlement` scheme, EURC and all non-USDC ERC-20 payments via Permit2, and presumably 7702-dispatched txs with non-standard outer-tx selectors. Treat these numbers as a **strict lower bound** on the ecosystem, not a snapshot of it. The implied average payment size of ~$4 below is an order of magnitude above the ~$0.30–0.90 ecosystem average reported by reproducible third-party sources — strong evidence of long-tail under-capture.

### What we captured

- **Daily volume:** ~60–80K txs/day in our index, peak ~310K USDC on a single day (2026-04-16)
- **Cumulative volume:** ~$15.3M USDC
- **Unique payers:** ~39K
- **Unique recipients:** ~19K
- **Distinct `tx.from` senders observed:** 500+ (vs. 22 in our curated registry) — lower bound per coverage caveat above
- **Top 5 senders:** each routed 590K–600K transactions (highly concentrated)
- **Payment size:** dust (0.001 USDC) → ~60 USDC, with a long tail of round-number micro-payments
- **Outer-tx shapes:** majority via direct `transferWithAuthorization` (classic-sig, `0xe3ee160e`) and the unattributed `0x93d9c747` (see Appendix A); Multicall3 minority but growing. **The bytes-overload `0xcf092995` is not captured by our filter at all** — its share is unknown to us but suspected dominant per the 97/3 split in Section 2.

### External anchors for the new project

Use these to calibrate your own indexer's coverage. Listed in rough order of reproducibility:

| Source | What it gives you | Why anchor against it |
|---|---|---|
| `x402scan.com` (Merit Systems) | Purpose-built x402 block explorer, includes a facilitators page (`x402scan.com/facilitators`) and a public SQL-style query API | Most reproducible primary source; built specifically for x402; the explorer the protocol's own ecosystem points to |
| Dune `@hashed_official/x402-analytics` | SQL aggregation across known facilitators; queries are inspectable on Dune | Inspectable methodology; widely cited; you can read and re-run the underlying SQL |
| `facilitators.x402.watch` (community-maintained) | 19 facilitators × 94 addresses across Base/Solana/Polygon, with block-explorer links | The publicly-listed cdp.coinbase.com operator endpoint; canonical facilitator anchor set |

### Third-party totals as a sanity check

Roughly in order of credibility / reproducibility:

- **Dune `@hashed_official` via Stacy Muur snapshot (Nov 28, 2025):** ~40M total x402 tx over two months, 95% on Base, 98.6% USDC; daily peak ~3M tx (Nov 2-3); stable at 1-2M daily late Nov; ~$0.90–1.00 average payment on EVM. ⚠ Snapshot read from a tweet, the underlying Dune query is inspectable.
- **PANews (Oct 26, 2025), Dune-sourced:** "Coinbase Facilitator (Base) is the largest payment processing service, processing over $1.004 million in cumulative transaction volume and over 1.16 million transactions"; "PayAI (Base and Solana) has processed over $219,000 in cumulative transaction volume and over 175,000 transactions"; "buyers: 74,000…sellers: 1,405."
- **Cointelegraph (Oct 22, 2025), Dune-cited:** ~500K x402 transactions Oct 14–20 alone, +10,780% vs four weeks earlier; record day 239,505 tx.
- **Phemex News citing Decentralised.Co:** "x402's network of facilitators has processed over 18.82 million transactions since its launch, marking a 35-fold increase since May…82% of transactions occur via Coinbase and Daydreams." ⚠ Secondhand aggregation; date unclear.
- **Sherlock (March 2026):** "x402 has processed over 119 million transactions on Base and 35 million on Solana, handles roughly $600 million in annualized volume." ⚠ Doesn't cite its primary source. Treat as a downstream aggregator, not an anchor. The "35M" figure in the same paragraph appears as both Solana tx count and Base dollar value — easy to misread but not a contradiction.
- **OAK Research:** Useful context — "USDT, the largest stablecoin by market cap, does not use EIP-3009 and has no plans to adopt it. In short, around 40% of the entire stablecoin market remains incompatible." Reinforces that an EIP-3009-only indexer is single-token-family, not single-protocol.

### Implied coverage gap

Our index: ~3.7M tx / ~$15.3M over Jan–Apr 2026.

Whichever third-party number you anchor against (Dune ~40M total tx by end of Nov 2025; Sherlock 119M on Base by March 2026; Phemex 18.82M cumulative; OAK and PANews supporting), our capture is **at least an order of magnitude below** the public numbers by tx count. By dollar volume the gap is smaller (we caught roughly 40-50% if Sherlock is right). This is the shape of "caught the big EOA-classic-sig payments, missed the long tail of micropayments and the entire Permit2 path."

When you re-index in the new project, the headline metric to chase is: **does your tx count converge to within ~2× of `x402scan` or Dune `@hashed_official` for the same time window?** If not, you're still missing a data path.
