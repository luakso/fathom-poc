# x402 Indexing — Fundamentals

A teaching companion to [`x402-indexing-findings.md`](./x402-indexing-findings.md). The findings doc leans on a handful of building blocks — the EVM, ERC-20, EIP-3009, log indices, sighashes, reorgs, archive nodes, and Permit2. The primer below gives each one a single-paragraph definition, ordered so each concept leans on the previous ones. Parts 1–12 then expand the same concepts in depth, so the findings doc reads as analysis rather than as a wall of jargon.

Read top-to-bottom on first pass. Skip back to the relevant chapter when you need to refresh a single concept. A glossary lives at the end.

---

## A 60-second primer

Each definition below is deliberately terse — every concept is unpacked in detail later (cross-references in parentheses point to the deeper section). The ordering is chosen so each entry leans on the previous ones.

**EVM (Ethereum Virtual Machine)** — The computation engine that every Ethereum node runs. It's a deterministic state machine: given the current state and a transaction, every node computes the same next state. Smart contracts are bytecode that executes on the EVM. "EVM-compatible" chains — like Base, where x402 lives — run the same bytecode and tooling. (Part 1.)

**ERC-20** — The token standard. It's an agreed-upon interface (a set of function signatures like `transfer`, `balanceOf`, `approve`, `allowance`) that a contract implements so wallets and other contracts can treat any token uniformly. USDC is an ERC-20 token. The standard also defines events, notably `Transfer` and `Approval`. (§2.1.)

**EIP-3009 ("Transfer With Authorization")** — An extension that lets someone authorize a token transfer via an off-chain signature, which a third party then submits on-chain. The signer never sends the transaction themselves (so they need no gas/ETH). This is the mechanism x402 leans on: a payer signs an authorization, and a facilitator submits it. The relevant functions are `transferWithAuthorization` and `receiveWithAuthorization`. (Part 3.)

**Log indices** — When a transaction executes, contracts emit events, which are recorded as logs. Each log has a position: a `log_index` (its ordinal within the block, zero-indexed) plus the transaction hash and block number. Indexers use this tuple to uniquely identify and deduplicate events — essential when you're tailing a chain and might see the same log twice. (§1.5, Part 7.)

**Sighash (function selector)** — The first 4 bytes of the Keccak-256 hash of a function's signature (e.g. `transfer(address,uint256)` → `0xa9059cbb`). It's how the EVM knows which function a transaction is calling — the calldata begins with this selector. For event logs, the analogous thing is the 32-byte `topic0`, the hash of the event signature. Verifying you're filtering on the correct selector/topic is exactly the kind of on-chain verification the findings doc covers. (§1.4, Part 6.)

**Reorgs (chain reorganizations)** — Occasionally the chain's "tip" gets rewritten: a block you thought was canonical gets replaced by a competing block, and its transactions (and their logs) may vanish or change order. An indexer that already wrote those logs must be able to detect and roll them back. This is the central correctness hazard for live-tail indexing. (§1.7, Part 9.)

**Archive nodes** — A normal ("full") node prunes old intermediate state to save space. An archive node retains the complete historical state, so you can query balances or call contracts as of any past block. Historical backfilling (or anything needing old state) requires archive access — which is why streaming services like Envio HyperSync matter for historical indexing. (Part 8.)

**Permit2** — A Uniswap-deployed contract (`0x000000000022D473030F116dDEE9F6B43aC78BA3`) that offers a single, universal approval/signature system across all tokens, working around the fact that not every ERC-20 implements signature-based approvals (EIP-2612/3009). It's a different signing scheme from EIP-3009 — relevant because, when classifying on-chain payment flows, you need to distinguish EIP-3009 authorizations from Permit2-mediated transfers so you don't conflate or miscount them. (§4.4, Part 12.)

---

## How this document is organized

1. **Part 1 — The EVM, briefly.** What a chain is, what a transaction is, what events are, what RPC is. The bare floor you stand on.
2. **Part 2 — Tokens and meta-transactions.** ERC-20, USDC, why "anyone can submit a signed payment" is a non-trivial problem.
3. **Part 3 — EIP-3009.** The mechanism that x402's `exact` scheme is built on. The bytes-signature overload. The look-alike functions that aren't x402.
4. **Part 4 — The x402 protocol.** Three actors, three schemes, what Permit2 and EIP-7702 are doing on the periphery.
5. **Part 5 — Indexing first principles.** What "indexing" actually means and why it's a two-step join.
6. **Part 6 — The filtering puzzle.** Why subscribing to the event isn't enough, and why a single-sighash filter cuts the wrong way.
7. **Part 7 — The pairing puzzle.** Multicall and the `log_index` rule.
8. **Part 8 — Two data paths.** RPC vs streaming archive, and why one cursor binds them.
9. **Part 9 — Reorgs and idempotency.** Why we lag, and why the primary key matters.
10. **Part 10 — Storage.** SQLite-specific things that bite.
11. **Part 11 — Facilitator discovery.** Why there's no registry and what to anchor against.
12. **Part 12 — Coverage gaps.** The schemes we don't index, what we miss, and why it matters.
13. **Glossary.**

---

# Part 1 — The EVM, briefly

## 1.1 Chains, blocks, transactions

A **blockchain** is an append-only log of state changes. On Ethereum-style chains, the log is grouped into **blocks**, each containing an ordered list of **transactions**. A transaction is a signed message that asks the network to update state — transfer ETH, deploy a contract, call a function on an existing contract. Once a block is accepted, every honest node agrees on the post-block state.

**Base** is a Layer-2 chain built on Ethereum by Coinbase. For day-to-day indexing it looks and behaves like Ethereum mainnet, with two practical differences:

- **Blocks are much faster.** Base produces a new block roughly every 2 seconds (vs. ~12s on mainnet). This means more blocks per day, smaller per-block tx counts, and tighter latency budgets for live syncs.
- **A centralized sequencer orders transactions.** Coinbase runs the sequencer that decides transaction inclusion order. This makes Base cheap and fast but means reorg safety is operational (sequencer policy) rather than purely cryptographic. In practice deep reorgs are rare; in worst cases (sequencer restart, multi-hour outage) they have happened.

Key numbers to internalize:

| Property | Base | Why it matters |
|---|---|---|
| Chain ID | `8453` | Identifies the chain in signatures and tooling |
| Block time | ~2 seconds | Influences confirmation depth and indexer latency |
| Gas token | ETH | Same as mainnet — facilitators need ETH balance |
| Block number for "Jan 1 2026" | `40,222,720` | Our chosen genesis for indexing x402 |

## 1.2 Accounts: EOAs vs contracts

There are exactly two kinds of accounts on the EVM:

- **Externally Owned Accounts (EOAs).** Controlled by a private key. The account address is derived from the key's public counterpart. EOAs are what humans (and most bots) hold. Only an EOA can *initiate* a transaction.
- **Contract accounts.** Hold bytecode. They don't have a private key — they execute their code when called. They cannot initiate transactions on their own; they only run when something (an EOA or another contract) calls them.

The address format is identical for both: a 20-byte hex string like `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913`. You can tell them apart by calling `eth_getCode(address)`:

- EOA → returns `0x` (empty bytecode)
- Contract → returns the deployed bytecode

This distinction matters for x402: the **agent** is usually an EOA (a private key the agent owns), the **USDC contract** is a contract account, the **facilitator** may be either — most are EOAs operated by a service, but some are dispatcher contracts.

**Smart contract wallets (SCWs)** are contracts that *act like* wallets — they hold funds, authorize transfers, can sign things using EIP-1271 (see §3.4). From the chain's perspective they're contracts, but functionally they're user wallets. We'll come back to this.

**EIP-7702 (Pectra upgrade, 2025)** adds a wrinkle: an EOA can *temporarily delegate* its execution to contract bytecode during one transaction. So during execution, an EOA can behave like a contract — running arbitrary logic, doing batch operations — and then revert to being an EOA. This is how some facilitators implement "gasless" UX. We'll come back to this in §4.5.

## 1.3 Transactions: structure, gas, signatures

A transaction is a signed message with these fields (simplified — modern EIP-1559 form):

| Field | Meaning |
|---|---|
| `from` | The EOA submitting and paying for the transaction |
| `to` | Target address (contract or EOA). `null` for contract deployments |
| `nonce` | Per-sender sequence number. Must be exactly `previous + 1` |
| `value` | ETH being transferred to `to`, in wei (10^-18 ETH) |
| `gas_limit` | Maximum gas the tx is allowed to consume |
| `max_fee_per_gas` | Maximum gas price the sender will pay (wei per gas unit) |
| `max_priority_fee_per_gas` | Tip going to the block proposer (Base: sequencer) |
| `input` (a.k.a. `data` / `calldata`) | Arbitrary bytes — typically an ABI-encoded function call |
| `chainId` | The chain this tx is valid on (replay protection) |
| Signature `v, r, s` | Proves `from` signed this exact transaction |

**Gas** is the EVM's metering mechanism. Every operation costs gas; the sender pays `gas_used × effective_gas_price` in ETH regardless of whether the call succeeded. Out-of-gas mid-execution reverts state but still charges. This is why "who pays gas" is a critical question — it means an EOA needs ETH before it can do anything useful, including transferring USDC.

**Three transaction types coexist on modern chains:**

| Type | Name | Pricing |
|---|---|---|
| 0 | Legacy | Single `gas_price` field |
| 1 | EIP-2930 | Adds access lists; rare in practice |
| 2 | EIP-1559 | Base fee + priority tip — what almost everyone uses now |

We store `tx_type` per row because the pricing model affects how to compute and reason about gas cost.

The sender signs a hash of all the above fields except the signature, recovers `from` from the signature, and broadcasts. Once mined, the transaction is identified by its **transaction hash** (`tx_hash`) — keccak256 of the signed payload. `tx_hash` is the canonical identifier you use everywhere.

## 1.4 Calldata and method selectors

The `input` field of a transaction is raw bytes. When the `to` address is a contract, the EVM hands the bytes to the contract's code and runs them. By convention (the **ABI** — Application Binary Interface), Solidity contracts expect the bytes laid out as:

```
[ 4 bytes: method selector ][ N bytes: ABI-encoded arguments ]
```

The **method selector** (often called a **sighash** in indexer slang) is the first 4 bytes of the keccak256 hash of the function's canonical signature:

```
selector = keccak256("transfer(address,uint256)")[0..4]
        = 0xa9059cbb
```

The signature is the function name plus parenthesized parameter types — no parameter names, no spaces. The keccak hash is taken over the ASCII bytes.

You can compute this independently — don't trust prose attributions. The findings doc has a section called "Compute selectors instead of trusting prose attributions" precisely because a prior version of the document mis-identified `0xcf092995` based on someone's notes, and an independent recomputation caught the error.

Useful tooling:

- [4byte.directory](https://www.4byte.directory) — a community database of `selector → signature` mappings. Most known selectors are there, but the directory is contributed and has duplicates/conflicts; treat it as a starting point, not gospel.
- Any chain-explorer's "decode" button on a verified contract — Basescan, Etherscan, etc.
- A one-liner in your language: take `keccak256(signature_string)[0..4]`.

Why this matters for indexing: a transaction's `input[0..4]` tells you which function the sender intended to call on the `to` contract. **You can filter on this without decoding the rest.** That's the whole point of the "sighash filter" mentioned throughout the findings doc.

## 1.5 Events (logs): topics and `log_index`

When a contract wants to emit information that doesn't change state but should be discoverable later, it emits an **event** (Solidity term) — at the EVM level this is a `LOG` opcode that writes a **log entry** to the transaction's receipt. After a transaction executes successfully, its receipt contains:

- `status` — success/failure
- `gas_used` — actual gas consumed
- `logs` — ordered list of log entries

Each log entry has:

| Field | Meaning |
|---|---|
| `address` | The contract that emitted the log |
| `topics` | Up to 4 indexed values, each 32 bytes. `topics[0]` is the **event signature hash** (keccak256 of the event signature). `topics[1..]` are indexed parameters in order. |
| `data` | ABI-encoded non-indexed parameters, packed together |
| `block_number`, `tx_hash`, `tx_index`, `log_index` | Where this log sits in chain history |

The **event signature** follows the same shape as a function signature:

```
"Transfer(address,address,uint256)"
keccak256(...) = 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
```

That hash is what you filter on when calling `eth_getLogs` — see §1.6.

**`log_index` is critical.** It's the **0-indexed position of this log within its receipt** — that is, within the parent transaction. Two log entries in the same transaction have the same `tx_hash` but different `log_index`. A single transaction can emit dozens of logs (think Multicall settling many payments at once). This is why the natural primary key for indexed events is `(chain, tx_hash, log_index)`, not `tx_hash` alone — see §10.2 and Section 7 of the findings doc.

The distinction between **indexed** and **non-indexed** event parameters is purely an indexing optimization in Solidity:

- Indexed parameters go into `topics[1..]`. You can filter on them server-side via RPC.
- Non-indexed parameters get packed into `data`. You can't filter on them — you fetch the log and decode locally.

Solidity allows up to 3 indexed parameters per event. The event signature hash itself eats `topics[0]`.

## 1.6 RPC: the request menu

To talk to a chain you talk to a **node** — software that maintains the chain state and serves queries. Most developers don't run their own; they use a hosted **RPC provider** (Alchemy, Infura, QuickNode, etc.). The provider runs nodes and exposes the **JSON-RPC** interface defined by Ethereum.

The handful of RPC methods you actually need for indexing:

| Method | Returns | When you use it |
|---|---|---|
| `eth_blockNumber` | Current head block number | Polling for new blocks |
| `eth_getLogs(filter)` | All log entries matching the filter | Primary discovery query |
| `eth_getBlockByNumber(n, full)` | Block header + (optionally) full transaction objects | Joining logs back to their parent txs |
| `eth_getTransactionByHash(h)` | One transaction | Per-tx join (the N+1 trap — see §5.2) |
| `eth_getTransactionReceipt(h)` | One receipt with logs and gas | Per-tx data fetch |
| `eth_getBlockReceipts(n)` | *All* receipts in a block | Block-level fetch — much fewer round-trips |
| `eth_call` | Read-only contract call result | Looking up token decimals, calling view functions |
| `eth_getCode(address)` | Bytecode at an address | Distinguish EOA from contract |

A **log filter** for `eth_getLogs` looks like:

```
{
  fromBlock: 40_222_720,
  toBlock:   40_222_820,
  address:   USDC_CONTRACT,          // single address or list
  topics: [
    AuthorizationUsed_topic_hash,    // topics[0] — event signature
    null,                            // topics[1] — wildcard
    null                             // topics[2] — wildcard
  ]
}
```

Within a `topics` array slot you can pass `null` (wildcard) or a value to match, or an array (OR). Across slots is AND. Server-side filtering by topic is dramatically cheaper than fetching everything and filtering client-side.

**Provider limits are the gravity in this space.** Free tiers cap requests per second, response sizes, and block ranges per call. For example, Alchemy's free tier limits `eth_getLogs` to ~10-block ranges in practice. These limits drive the entire architecture of §5 (RPC for live tail, archive for backfill).

## 1.7 Reorgs and confirmation depth

A **reorg** (reorganization) happens when two valid blocks compete at the same height and the network eventually agrees on one of them — temporarily, the "losing" block (and any transactions only in it) gets dropped from canonical history. On mainnet this is rare (1-2 blocks deep, occasionally); on Base it's even rarer for normal operation thanks to the centralized sequencer, but it can happen during sequencer restarts.

A naïve indexer that writes blocks the instant they appear will record transactions that later get reorged out, leaving phantom rows. The standard defense is **confirmation depth**: only treat blocks as final once they're at least N blocks behind the tip. For x402 on Base we use:

- **Live tail: 6 blocks behind tip** (~12 seconds latency on Base). Adequate for normal operation.
- **Historical backfill: 500 blocks behind tip.** Long-running streams can race against reorgs at the leading edge; 500 is cheap insurance.

If you want lower live latency you have to actually detect reorgs (track block hashes, roll back on mismatch). We don't — the 6-block buffer suffices for the analytics use case.

---

# Part 2 — Tokens and meta-transactions

## 2.1 ERC-20 in one page

**ERC-20** is the standard interface for fungible tokens on Ethereum-style chains. A token is just a contract that tracks balances:

```
mapping(address => uint256) balances;
mapping(address => mapping(address => uint256)) allowances;
```

The required functions are roughly:

- `balanceOf(account) → uint256` — read balance
- `transfer(to, amount) → bool` — move tokens from `msg.sender` to `to`
- `transferFrom(from, to, amount) → bool` — move tokens from `from` to `to`, decrementing the caller's allowance
- `approve(spender, amount) → bool` — grant `spender` an allowance to pull `amount` from your balance
- `allowance(owner, spender) → uint256` — read current allowance

And the required events:

- `Transfer(from, to, value)` — emitted on every balance change
- `Approval(owner, spender, value)` — emitted on every allowance change

The `Transfer` event is the single most-indexed event on Ethereum. Its topic hash is worth memorizing:

```
keccak256("Transfer(address,address,uint256)")
  = 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
```

**Decimals.** `balanceOf` returns a raw integer; the human-readable amount is `raw / 10^decimals`. USDC uses **6 decimals**, so `1 USDC = 1_000_000 raw`. Always store raw and derive human readable, not the other way around — see §10.

## 2.2 USDC specifically

USDC is Circle's fiat-backed stablecoin. On Base it lives at:

```
USDC proxy: 0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913
```

That's a **proxy contract**. Behind the proxy is an **implementation contract** that holds the actual logic. The proxy forwards all calls to whatever implementation Circle has set. As of writing the implementation is `FiatTokenV2_2` at `0x2Ce6311ddAE708829bc0784C967b7d77D19FD779`.

This pattern matters for two reasons:

1. **You always send transactions to the proxy address**, never to the implementation directly. The proxy address is the stable identity.
2. **Circle can upgrade the implementation.** When they do, the proxy emits an `Upgraded(address)` event. New implementations can add functions (and selectors). FiatTokenV2_2 in November 2023 added the bytes-signature overloads we'll see in §3 — before that, those selectors didn't exist on USDC at all.

A defensive indexer watches `Upgraded(address)` on the proxy so it gets a heads-up before Circle changes the surface area underneath it.

## 2.3 The meta-transaction problem

The naïve flow for "agent pays merchant in USDC" is:

1. Agent calls `USDC.transfer(merchant, amount)`.
2. Network charges the agent gas in ETH.

This breaks down for autonomous agents:

- **Agents need ETH.** Even if they're paid in USDC, they need a separate ETH balance to send the transaction. ETH balance management is operational overhead.
- **Two-step approve-then-pull is worse.** If a third party is going to pull the payment (e.g., a merchant's billing system), the agent first calls `approve` (paying gas) and then the third party calls `transferFrom` (paying their own gas). Two transactions for one payment.
- **What if the agent has no ETH at all?** Dead in the water.

The general solution is a **meta-transaction**: the agent **signs** an authorization off-chain (no gas, no network), and someone else (the "relayer" or "facilitator") **submits** the on-chain transaction that consumes the authorization. The facilitator pays gas. The agent never touches the chain.

For this to work safely the on-chain contract must:

1. Verify the off-chain signature is genuinely from the agent.
2. Verify the authorization hasn't been replayed.
3. Verify it hasn't expired.

These three requirements are exactly what EIP-3009 standardizes for ERC-20 transfers.

---

# Part 3 — EIP-3009

## 3.1 What EIP-3009 adds to ERC-20

EIP-3009 is a small standard that adds three functions to an ERC-20 token:

| Function | Purpose |
|---|---|
| `transferWithAuthorization` | **Payer-push.** Agent signs "send X to recipient"; anyone can submit and the tokens move from agent to recipient. |
| `receiveWithAuthorization` | **Payee-pull.** Agent signs "let recipient pull X"; only the recipient can submit. Used when the recipient orchestrates. |
| `cancelAuthorization` | Agent invalidates a not-yet-used authorization by burning its nonce. |

x402's `exact` scheme uses `transferWithAuthorization`. The `receiveWithAuthorization` variant has the same shape but a different security model (only the named payee can submit) — it is **not** x402 and the indexer must explicitly exclude its selectors (see §6.3).

## 3.2 The signature shape

The agent signs (off-chain, no gas) an EIP-712 structured message:

```
TransferWithAuthorization {
  from:        address  // the agent
  to:          address  // the recipient
  value:       uint256  // raw amount, e.g. 100_000 for 0.1 USDC
  validAfter:  uint256  // unix timestamp — earliest valid time
  validBefore: uint256  // unix timestamp — expires at
  nonce:       bytes32  // random; identifies this authorization
}
```

The signature is bound to a specific token contract (via the EIP-712 domain separator), so it can't be replayed against a different contract. The `nonce` here is **not** the agent's transaction nonce — it's a per-authorization random identifier. Reusing the same nonce after consumption fails.

To submit on-chain, the facilitator calls one of two overloads on USDC:

```
// Classic-signature overload — original FiatTokenV2
transferWithAuthorization(
  address from, address to, uint256 value,
  uint256 validAfter, uint256 validBefore, bytes32 nonce,
  uint8 v, bytes32 r, bytes32 s
)
// selector: 0xe3ee160e

// Bytes-signature overload — added in FiatTokenV2_2 (Nov 2023)
transferWithAuthorization(
  address from, address to, uint256 value,
  uint256 validAfter, uint256 validBefore, bytes32 nonce,
  bytes signature
)
// selector: 0xcf092995
```

Same arguments either way. **Same on-chain effect. Same events emitted.** The only difference is how the signature is packaged.

The contract:
1. Reconstructs the EIP-712 hash from `from`, `to`, `value`, …
2. Recovers the signer from the signature
3. Checks `signer == from`
4. Checks `nonce` hasn't been used or canceled for `from`
5. Checks `validAfter ≤ block.timestamp < validBefore`
6. Transfers `value` from `from` to `to`
7. Marks the nonce as used
8. Emits `Transfer(from, to, value)` and `AuthorizationUsed(authorizer=from, nonce)`

## 3.3 Why two overloads?

The classic `(v, r, s)` overload assumes the signer is an **EOA** producing a 65-byte ECDSA signature, neatly split into 32-byte `r`, 32-byte `s`, and 1-byte recovery `v`. This works fine for human or bot wallets backed by private keys.

The bytes overload was added for **EIP-1271** support (see §3.4): contract wallets sign by calling a method on themselves, returning an opaque byte string that's not a standard ECDSA signature. The bytes overload accepts that.

Crucially, the bytes overload **also works for EOAs**. The caller just packs the EOA's `r || s || v` into the bytes argument. The contract's `_isValidSignature` function tries ECDSA recovery first, and if the signer is a contract, falls back to EIP-1271.

**The Coinbase TypeScript reference facilitator prefers the bytes overload across the board** — for both EOA-packed-ECDSA and EIP-1271 SCW signatures. This is because the bytes overload is the more general code path. The `x402-rs` implementation historically preferred the classic overload, which is the source of github issue #26 in their repo ("frequently reverts 'invalid signature'") and a known compatibility gotcha.

**The consequence for indexers:** the selector (`0xe3ee160e` vs `0xcf092995`) tells you the *call shape* — which overload the facilitator chose — but **not** the wallet type of the payer. An EOA payer's payment can show up under either selector depending on which facilitator submitted it. If you want to know whether a payer is an EOA or a smart contract, you must call `eth_getCode(authorization.from)` — and note that `authorization.from` (the payer) is decoded from calldata, while `tx.from` (the facilitator) is something else entirely (see §5).

Filtering by `sighash == 0xe3ee160e` only — which the project did at first — drops the entire bytes-overload call shape. Per the findings doc this is probably the majority of x402 traffic by tx count.

## 3.4 EIP-1271 — contract wallet signatures

Standard ECDSA signatures only work for EOAs because an EOA owns the private key. A **smart contract wallet** (SCW) — like Safe (formerly Gnosis Safe), Argent, the Coinbase Smart Wallet, account-abstraction wallets — has no private key. The "wallet" is a contract; ownership is encoded in the contract's logic (multisig, social recovery, passkey verification, whatever).

**EIP-1271** standardizes how contracts signal "this signature is valid":

```solidity
interface IERC1271 {
  // Returns the magic value 0x1626ba7e if the signature is valid for the hash.
  function isValidSignature(bytes32 hash, bytes signature) external view returns (bytes4);
}
```

When a token contract receives a bytes signature claimed to be from address `from`, and `from` is a contract, the token contract calls `from.isValidSignature(hash, signature)` and checks for the magic return value. If yes, the signature is accepted. The `signature` bytes can be anything the wallet's logic understands — multisig aggregation, passkey assertion, whatever.

The practical upshot for x402 is that **a payer can be a smart-contract wallet** (very common — many onboarding flows use SCWs to skip the seed-phrase UX) and the indexer needs to handle bytes signatures gracefully. The bytes overload (`0xcf092995`) is the one that supports this path, but as §3.3 said, the bytes overload is also used for EOAs.

## 3.5 The events emitted

After a successful settle, USDC emits two log entries (in this order, within the same transaction receipt):

```
Transfer(
  from  = authorization.from       // the payer
  to    = authorization.to         // the recipient
  value = authorization.value      // raw USDC
)
topic0 = 0xddf252ad...

AuthorizationUsed(
  authorizer = authorization.from  // the payer
  nonce      = authorization.nonce // the auth's random id
)
topic0 = 0x98de5035...
```

`Transfer` is from the standard ERC-20 surface. `AuthorizationUsed` is the EIP-3009 marker that this `Transfer` was a meta-transaction settlement, not a regular `transfer` call.

Indexing on `AuthorizationUsed` topic is what lets you find EIP-3009 settlements without also picking up ordinary USDC transfers. But — as §6 will explore — the event alone doesn't tell you it was *x402* specifically.

## 3.6 The look-alike: `receiveWithAuthorization`

`receiveWithAuthorization` has the same arguments and emits the same events. It also emits `AuthorizationUsed`. The only difference is a single line in the implementation: it requires `msg.sender == to`. The named recipient must be the one submitting.

This is **payee-pull**: the recipient orchestrates the transfer. There is no third-party facilitator. **It is not x402.**

For an indexer this means:

- Filtering on `AuthorizationUsed` topic alone captures both `transfer*` and `receive*` flows.
- The two `receiveWithAuthorization` selectors (`0xef55bec6` classic, `0x88b7ab63` bytes) must be **excluded** post-hoc.

This is the "explicit-exclude payee-pull" piece of the recommended filter in §6.4.

## 3.7 `cancelAuthorization` and the canceled state

An agent can invalidate a not-yet-used authorization by calling `cancelAuthorization`. This consumes the nonce in a different terminal state ("canceled" instead of "used"). The event emitted is `AuthorizationCanceled`, not `AuthorizationUsed`:

```
keccak256("AuthorizationCanceled(address,bytes32)")
  = 0x1cdd46ff242716cdaa72d159d339a485b3438398348d68f09d7c8c0a59353d81
```

A topic filter on `AuthorizationUsed` doesn't see cancellations at all. Volume is probably small; for our analytics we currently ignore it. If you model nonces as a state machine — `created → (used | canceled | expired)` — canceled is its own terminal state, semantically distinct from used.

---

# Part 4 — The x402 protocol

## 4.1 Why x402 exists

HTTP defines a status code, **`402 Payment Required`**, that has historically been a placeholder — no standard way to actually request and accept payment over HTTP. x402 is Coinbase's protocol that fills in the placeholder using on-chain stablecoin settlement.

The flow:

1. Client (an agent) makes an HTTP request to a service.
2. Service responds `402 Payment Required` with a payment request body (amount, recipient, accepted schemes, expiry).
3. Client signs an authorization for that payment off-chain and retries the request with the signed authorization in a header.
4. Service forwards the signed authorization to a **facilitator** — a backend service the merchant has a relationship with.
5. The facilitator submits the on-chain transaction that actually moves the money, then tells the merchant "yep, settled" so the merchant can return the paid response.

The protocol abstracts over multiple settlement **schemes** (see §4.4). The agent never directly touches the chain; the merchant never directly touches the chain; the facilitator handles all on-chain interaction.

## 4.2 The three actors

| Actor | Role | On-chain identity |
|---|---|---|
| **Payer** (a.k.a. agent) | Signs payment authorizations | `authorization.from` (inside calldata or event) |
| **Facilitator** | Submits the tx, pays gas | `tx.from` of the settlement transaction |
| **Recipient** (merchant) | Receives the payment | `authorization.to` / `Transfer.to` |

Every indexed row needs all three. The payer and recipient come straight from event data (`AuthorizationUsed.authorizer`, `Transfer.to`). **The facilitator does not appear in any event** — you have to join the log back to the parent transaction and read `tx.from`. This join is the entire reason §5/§7 of the findings doc exist.

## 4.3 Why the facilitator is a real actor (not just plumbing)

You might ask: if the facilitator just submits a signed message anyone could submit, why does it matter who they are?

- **They pay gas.** A high-volume facilitator absorbs real ETH cost. Indexing the per-facilitator tx count and gas spend is a legitimate analytics question.
- **They choose the call shape.** Bytes overload vs classic, single settlement vs multicall, plain settle vs settle-and-execute-something-else. Each facilitator's implementation choices affect what you see on-chain.
- **They aggregate trust.** A merchant trusts a specific facilitator's behavior (rate limits, fraud detection, payout latency). The set of operating facilitators is part of the protocol's social fabric.
- **They're the unit of competitive analysis.** "Who's processing the most x402 tx?" is a meaningful question. The answer right now is "Coinbase's CDP facilitator" by a wide margin.

This is why the data model treats `facilitator` as a first-class column and §11 talks at length about how to discover them.

## 4.4 The three settlement schemes

The x402 spec defines (or has shipped) at least three schemes. Only one — `exact` — is what most people mean when they say "x402."

### `exact` — fixed payment via EIP-3009

The default scheme. The agent signs an authorization for an exact amount, the facilitator submits `transferWithAuthorization` on the token contract. This is what every prior section described. Today this is shipped on **USDC** and **EURC** (which are both Circle stablecoins with EIP-3009 support).

For a non-Circle ERC-20 (USDT, DAI, anything else), `exact` is **not** available via EIP-3009 because most ERC-20s don't implement EIP-3009. To support non-Circle tokens via `exact`, Coinbase's facilitator uses a different mechanism — Permit2.

### `upto` — variable payment via Permit2

Some use cases need "let the merchant charge up to X" rather than "charge exactly X" — pay-per-call APIs, metered services, anything where the final amount isn't known when the authorization is signed. EIP-3009 doesn't support this (the signed amount is fixed). The `upto` scheme uses **Uniswap's Permit2** contract instead.

**Permit2** is a universal permission system for any ERC-20. Its canonical address on every EVM chain:

```
0x000000000022D473030F116dDEE9F6B43aC78BA3
```

The relevant function is `permitWitnessTransferFrom`. The agent signs a "permit" with an attached witness — a structured piece of data that constrains how the permit can be used. For x402, the witness encodes the recipient and the maximum amount. The facilitator submits with a specific amount up to the cap.

**Implications for an indexer:**

- **Different contract.** Permit2, not USDC.
- **Different events.** `SignatureTransfer` on Permit2, not `AuthorizationUsed` on USDC.
- **Different ABI.** `permitWitnessTransferFrom` decodes differently.
- **Token-agnostic.** Permit2 works for any ERC-20, so the token has to become a first-class column.

A USDC-and-`AuthorizationUsed`-only indexer captures **zero** `upto` traffic. To cover it, you add a second data source.

### `batch-settlement` — off-chain vouchers + on-chain redemption

The agent signs a voucher off-chain that says "I owe you up to X." The merchant accumulates many vouchers and periodically redeems them in a batch on-chain against an escrow contract. The on-chain footprint is one settlement event covering many vouchers; the per-payment activity is off-chain entirely.

This is great for very high-frequency, very low-value payments where individual on-chain settlement would be uneconomical. It does **not** emit USDC `AuthorizationUsed` and uses custom escrow events. Currently the indexer doesn't touch this — see Section 12 of the findings doc.

## 4.5 EIP-7702 — gasless via temporary delegation

EIP-7702 (shipped in the Pectra upgrade, May 2025) lets an EOA temporarily authorize its account to execute as a specified contract's code during a single transaction. The EOA is still the `tx.from`, still pays gas, but during execution the EVM treats the EOA's address as if it had the delegated contract's bytecode.

In x402 land this is used by some facilitators (notably thirdweb's) for "gasless" submission patterns. The outer transaction's `tx.input[0..4]` reflects the delegation contract's dispatch function, not `transferWithAuthorization` — but the *inner* call to USDC's `transferWithAuthorization` still emits `AuthorizationUsed` from the USDC contract.

**For an indexer:** the topic filter still catches the event. The outer-tx sighash filter does **not** match `0xe3ee160e` or `0xcf092995` — it matches whatever the dispatcher uses. This is one of the reasons the findings doc cautions against single-sighash filtering and recommends a permissive allow-list. The unattributed selector `0x93d9c747` observed in our data is suspected (but unverified) to be a thirdweb EIP-7702 dispatcher.

You should **never** filter by transaction `type` to detect 7702 — type 4 vs 2 doesn't help you classify x402 vs non-x402. `AuthorizationUsed` is the right gate; the type and outer sighash are descriptive metadata.

---

# Part 5 — Indexing first principles

## 5.1 What "indexing" actually means

The chain itself is queryable — you can call `eth_getLogs` and get a list of events. But:

- **It's slow.** Each request takes a network round trip; free tiers throttle aggressively; per-request block ranges are small.
- **It's read-only.** You can't join, group, aggregate, or persist derived state.
- **It's stateless.** No "where did I leave off" cursor.
- **It's not easily decoded.** Logs come back as raw topics + data; you have to decode against an ABI.

An **indexer** is a small program that does the following loop forever:

1. Discover new log entries that match a filter.
2. Pull the parent transaction and block context needed to enrich them.
3. Decode the bytes into structured fields.
4. Insert into a local database (ours is SQLite).
5. Advance a cursor so the next run picks up where this one left off.

After indexing, you query the database — SQL, indexes, joins, aggregates — at local-disk speed. The chain is no longer in the hot path. This is what `x402-analysis status` is doing when you run it.

## 5.2 The two-step join

Events tell you **what happened**. Transactions tell you **who initiated**. Both are needed.

The naïve approach is one RPC call per log to fetch its parent:

```
logs = eth_getLogs(USDC, AuthorizationUsed, [from, to])
for each log:
    tx      = eth_getTransactionByHash(log.tx_hash)     // 1 RPC per log
    receipt = eth_getTransactionReceipt(log.tx_hash)    // 1 RPC per log
```

This is the **N+1 problem** familiar from web app data access — for N logs you do 2N follow-up requests. With a free RPC tier and dense blocks this burns through quotas in minutes.

The fix is **block-level batching**: fetch the whole block (including all its transactions) in one call, build an in-memory `tx_hash → tx` map, and join locally:

```
logs = eth_getLogs(USDC, AuthorizationUsed, [from, to])
distinct_blocks = unique(log.block_number for log in logs)

for block in distinct_blocks:                       // parallelism = 10
    full_block = eth_getBlockByNumber(block, full=true)

// Now filter the candidate txs by sighash before fetching receipts.
candidate_txs = txs whose hash appears in logs
                 AND whose input[0..4] matches our sighash allow-list

// Only fetch receipts we actually need.
for block in blocks_containing(candidate_txs):
    receipts = eth_getBlockReceipts(block)

// Decode + insert.
```

The savings are dramatic on dense blocks (one block fetch vs. N log fetches), and the sighash pre-filter avoids paying for receipts on blocks whose interesting txs are all non-x402.

This is the algorithm the live syncer runs. See §3 of the findings doc for the trade-offs.

## 5.3 Why this is a join at all

You might wonder: why isn't the facilitator address in the event? Two reasons:

1. **EIP-3009 was designed before x402.** The event was never intended to identify a "facilitator" — that's an x402-specific concept layered on top.
2. **There's no `msg.sender` field in the `AuthorizationUsed` event.** Solidity events emit what the contract chooses to emit. USDC chose `(authorizer, nonce)`. You can't retroactively add fields to an existing event without breaking everything.

So the indexer reconstructs the facilitator-payer-recipient triple from two sources: the log (payer, recipient via companion Transfer, amount, nonce) and the parent tx (`tx.from` = facilitator, plus useful metadata like the sighash, gas spent, etc.).

---

# Part 6 — The filtering puzzle

## 6.1 Why "just filter on the event" doesn't work

Naïve approach: `eth_getLogs(USDC, AuthorizationUsed_topic)`. Done, right?

No. Recall §3 — `AuthorizationUsed` is emitted by **every** EIP-3009 settle path on USDC, including:

- `transferWithAuthorization(...,v,r,s)` — x402, classic sig
- `transferWithAuthorization(...,bytes)` — x402, bytes sig
- `receiveWithAuthorization(...,v,r,s)` — **not** x402 (payee-pull)
- `receiveWithAuthorization(...,bytes)` — **not** x402 (payee-pull)
- Any wrapper contract that calls one of those internally

The event itself does not classify which entry point produced it.

## 6.2 Why "filter event + single sighash" also doesn't work

The next instinct is to combine the topic filter with a sighash filter on the parent transaction:

```
event topic == AuthorizationUsed
AND tx.input[0..4] == 0xe3ee160e   // classic transferWithAuthorization
```

This is what the project shipped first. The problem: **it excludes the bytes-overload call shape** (`0xcf092995`), which the dominant Coinbase TypeScript facilitator uses by default. Per §3.3 the bytes overload is *not* a smart-wallet indicator — it's the facilitator's preferred general-purpose code path. EOA-signed payments routed through it look identical to SCW-signed payments at the selector level.

Our own data, captured *with* the single-sighash filter, suggested that if we removed the sighash filter the breakdown would be roughly:

| Outer-tx sighash | Share (our index, ⚠ unverified externally) |
|---|---|
| `0xcf092995` (bytes) | ~97% |
| `0xe3ee160e` (classic) | ~3% |

⚠ — These numbers come from project design notes and are not externally verified. The right move is to re-probe against `x402scan.com` or a fresh HyperSync probe with no sighash filter. But even if the split is "only" 70/30 or 50/50, dropping one entire call shape is a massive coverage hole.

This is the single biggest finding in the whole findings doc. The first draft of that doc mislabeled `0xcf092995` as `receiveWithAuthorization`, then the second draft over-corrected by calling it "the SCW-payer path." Neither was right. The selector classifies the *call shape* (which overload the facilitator chose), not the *payer wallet type*.

## 6.3 Wrappers and Multicall

Even when you accept both transferWithAuthorization overloads, you miss payments routed through wrappers:

- **Multicall3** (`0xcA11bde05977b3631167028862bE2a173976CA11`) lets a single transaction batch many sub-calls. A facilitator can settle N x402 payments in one transaction by calling `Multicall3.aggregate3(...)` with N inner calls to USDC's `transferWithAuthorization`. The outer tx's sighash is `0x82ad56cb` (aggregate3), not any USDC selector. The inner calls still emit `AuthorizationUsed` from USDC — so the topic filter catches them — but the outer sighash filter rejects the parent tx.
- **Custom dispatchers / SettlementRouters.** Some facilitators run their own contract that wraps the USDC call (for logging, fee splitting, EIP-7702 delegation, etc.). Same shape: inner call emits the event, outer sighash is whatever the dispatcher uses. The unattributed `0x93d9c747` in our data falls into this category.

Each wrapper you allow lets some non-x402 back in. The defense is the **final gate**: even if you allow a wide sighash list, the `AuthorizationUsed` topic match is your guarantee that an EIP-3009 settlement actually happened.

## 6.4 The right filter shape

What you want is closer to:

```
KEEP IF:
    log.topic[0] == AuthorizationUsed
    AND parent_tx.input[0..4] IN {
        0xe3ee160e,    // transferWithAuthorization classic-sig
        0xcf092995,    // transferWithAuthorization bytes-sig
        0x82ad56cb,    // Multicall3.aggregate3
        ...other wrapper selectors discovered from data...
    }
    AND parent_tx.input[0..4] NOT IN {
        0xef55bec6,    // receiveWithAuthorization classic-sig
        0x88b7ab63,    // receiveWithAuthorization bytes-sig
    }
```

In words:

1. **The event presence is the strict requirement.** It guarantees the contract did the EIP-3009 settle work.
2. **The sighash allow-list is permissive.** Widening it catches more wrappers; the explicit excludes catch the obvious payee-pull cases.
3. **You probably want to keep `method_selector` as a column anyway** so you can refine the filter retroactively. See §10.

A practical rollout: index *without* a sighash filter for a few thousand blocks, group-by the outer sighashes you see, decide which to allow / exclude. Then ship the filter. The findings doc calls this the "probe before run" pattern.

## 6.5 What we still don't cover even with the right filter

The "right filter" above improves capture of `exact` scheme on USDC. It still misses:

- **EURC** on `exact` scheme (different token address)
- **Any non-Circle ERC-20** via Coinbase's facilitator (uses Permit2, different event surface)
- **The entire `upto` scheme** (Permit2, different event surface)
- **The entire `batch-settlement` scheme** (escrow contract, custom events)

Each of those needs a second data path. See §12.

---

# Part 7 — The pairing puzzle

## 7.1 The two events live side-by-side

A single successful `transferWithAuthorization` produces two log entries within the same transaction receipt:

```
log_index = N   :  Transfer(from=payer, to=recipient, value=X)
log_index = N+1 :  AuthorizationUsed(authorizer=payer, nonce)
```

The `Transfer` carries the **amount** and **recipient** (which are not in the `AuthorizationUsed` event). To enrich the indexed row you need to pair the two.

For a transaction with a single payment, "the `Transfer` immediately before the `AuthorizationUsed`" works. But a Multicall transaction emits multiple settlement pairs interleaved:

```
log_index = 0  :  Transfer(A → A', 1 USDC)
log_index = 1  :  AuthorizationUsed(A, nonce_A)
log_index = 2  :  Transfer(B → B', 5 USDC)
log_index = 3  :  AuthorizationUsed(B, nonce_B)
```

## 7.2 The naïve rule that's wrong

A first instinct: "find the closest `Transfer` to this `AuthorizationUsed`."

For `AuthorizationUsed(B)` at log_index 3:

- Distance to `Transfer(A→A')` at log_index 0: 3
- Distance to `Transfer(B→B')` at log_index 2: 1

Closest wins → `Transfer(B→B')`. Correct in this case.

But for `AuthorizationUsed(A)` at log_index 1:

- Distance to `Transfer(A→A')` at log_index 0: 1
- Distance to `Transfer(B→B')` at log_index 2: 1

Tie. And if for any reason there's an unrelated `Transfer` *after* the `AuthorizationUsed` (settlement-router fee forwarding, internal accounting, etc.), absolute distance can pick the wrong one entirely.

## 7.3 The right rule

**For each `AuthorizationUsed` at log_index `K`, pair it with the USDC `Transfer` with the highest `log_index` strictly less than `K`.**

```
function pair_transfer(receipt_logs, auth_log_index):
    best = None
    for log in receipt_logs:
        if log.address != USDC:               continue
        if log.topic[0] != Transfer_topic:    continue
        if log.log_index >= auth_log_index:   continue   // must precede
        if best is None or log.log_index > best.log_index:
            best = log
    return decode_transfer(best)
```

Two things to internalize:

- **The companion `Transfer` always precedes its `AuthorizationUsed`** in the same internal call. The EVM emits in execution order.
- **A `Transfer` after the `AuthorizationUsed` is never the right match.** It belongs to a later payment in the same tx, or to unrelated accounting (fee forwarding, etc.). Don't fall through to it.

## 7.4 Primary key follows from this

Multicall puts multiple settlements in one tx. They share `tx_hash`. They differ in `log_index`. The primary key has to be:

```
PRIMARY KEY (chain, tx_hash, log_index)
```

Not `tx_hash` alone. `chain` is in the prefix because nothing about the data model is intrinsically single-chain; making it a first-class column from day one means a future multi-chain deployment doesn't require a schema migration.

This same PK gives you idempotent resume for free — see §9.2.

---

# Part 8 — Two data paths

A single data source can't do both jobs:

- **Backfill the historical chain from genesis to current tip** — millions of blocks, low latency tolerance, high request volume.
- **Stay caught up on new blocks in near-real-time** — small per-tick volume, latency-sensitive, infrequent.

The first is hopeless with RPC providers and trivially solved with an archive service. The second is the opposite. Use both.

## 8.1 Live tail — RPC

For the live tail, RPC works fine. The block-batching algorithm from §5.2 keeps round-trips proportional to the number of *blocks containing matches*, not the number of *logs*. On the x402 traffic shape (tens of thousands of tx per day), a free Alchemy tier handles this comfortably.

We poll for new blocks, run the discovery + join + decode loop, insert in a batch transaction, advance the cursor. Confirmation depth: 6 blocks behind tip.

## 8.2 Historical backfill — streaming archive (HyperSync)

For the initial backfill from chain genesis (block ~40M on Base for our "Jan 1 2026" cutoff) to current tip, RPC is the wrong shape. **Envio HyperSync** is one of several streaming archive services that solve this — others include Subsquid, sqd Network, etc. They:

- Maintain a pre-indexed copy of the chain optimized for filter-and-stream queries.
- Accept a single query specifying topic + address + sighash filters, plus the fields you need.
- Stream the matching data back as bundles of `(logs, transactions, blocks)` already joined together.
- Are dramatically cheaper than RPC for large block ranges.

The query model:

```
ONE query:
    logs.address          = USDC
    logs.topic0           = AuthorizationUsed
    transactions.sighash  IN allowed_sighashes   // server-side
    fields                = whatever the decoder needs
    block_range           = [chain_genesis, current_tip - 500]
```

The server streams response batches; the client decodes and inserts each batch. One TCP connection, one query, many response chunks. Bandwidth scales with matches, not with block range.

For x402 specifically, the server-side sighash filter is a big deal — restricting on `sighash = 0xe3ee160e` gives ~30× bandwidth reduction vs. fetching every `AuthorizationUsed` log. But per §6, that's too narrow. The right move for the new index is `sighash IN (0xe3ee160e, 0xcf092995)` — bandwidth saving shrinks, but coverage improves dramatically.

Confirmation depth for backfill: **500 blocks behind tip** (vs. 6 for live). Backfill is latency-insensitive; the buffer is cheap insurance against the long-running stream racing reorgs at the leading edge.

## 8.3 One cursor binds both paths

The two paths share a single cursor table:

```sql
CREATE TABLE sync_state (
    chain      TEXT PRIMARY KEY,
    last_block INTEGER NOT NULL
)
```

Both binaries read `sync_state` before starting. Both write it after each batch commits. The backfill stops at `tip - 500`. The live sync resumes from `cursor + 1` (or genesis if empty) and runs to `tip - 6`. No special handoff code. The shared cursor and idempotent inserts (next section) mean overlap is harmless.

## 8.4 The probe pattern

Every long-running data pull should have a dry-run mode that runs the exact same query but writes nothing — just counts events, rows, bytes, elapsed time, decode failures. For HyperSync we have a `probe` subcommand on the backfill binary. The first time you save yourself from kicking off a 6-hour run that turns out to be returning the wrong shape, the probe pays for itself.

---

# Part 9 — Reorgs and idempotency

## 9.1 Confirmation depth as the reorg defense

We don't do explicit reorg detection. We rely on confirmation depth: only treat blocks as final once they're enough deep behind the tip that reorgs at that depth are essentially zero-probability for our use case. **6 blocks live, 500 blocks backfill.** This is correct for analytics — a 12-second latency on the live tail is fine when the downstream consumers are humans running CLI commands.

A production indexer that needs lower latency has to track per-block hashes (not just block numbers), detect mismatches against the canonical chain, and roll back the affected rows. We don't need that — the confirmation buffer plus our analytics use case means we never observe a reorg in indexed data.

## 9.2 Idempotent resume

After a crash mid-batch:

- The cursor is still pointing at the last *fully committed* batch's max block (we advance the cursor only after all rows in the batch are committed and the DB transaction succeeds).
- On restart, we re-fetch logs starting from `cursor + 1`. Some logs we already inserted will reappear.
- Inserting them again is silently ignored by:

```sql
INSERT OR IGNORE INTO transactions (chain, tx_hash, log_index, ...) VALUES (...)
```

`INSERT OR IGNORE` checks the primary key. If `(chain, tx_hash, log_index)` already exists, the insert is dropped without an error. No dedup logic in the indexer.

**This is why the PK shape matters.** A PK of `tx_hash` alone would either reject the second authorization in a Multicall tx (wrong — we'd miss data) or, depending on the upsert semantics, overwrite the first row's data with the second (also wrong). `(chain, tx_hash, log_index)` is the right granularity: each (transaction, log position) is unique.

## 9.3 The cursor advancement bug

Subtle: in the streaming archive's response, an empty batch can arrive with `max_block = 0`. If you blindly write that to the cursor:

```python
set_sync_cursor(chain, batch.max_block)   # writes 0 — resets to genesis
```

…you destroy your progress. Always guard:

```python
if batch.max_block > 0:
    set_sync_cursor(chain, batch.max_block)
```

This is the kind of bug that loses you days of backfill. We hit it.

---

# Part 10 — Storage

We use SQLite. For a single-machine indexer with one writer and many readers, it's the right answer — zero ops, file-on-disk, fast for our volume. But the defaults are not what you want.

## 10.1 PRAGMAs that matter

```sql
PRAGMA journal_mode = WAL;       -- write-ahead log: concurrent readers + 1 writer
PRAGMA synchronous = NORMAL;     -- still crash-safe, ~10× faster commits than FULL
PRAGMA busy_timeout = 5000;      -- wait 5s when locked instead of failing immediately
PRAGMA temp_store = MEMORY;      -- keep temp tables and GROUP BY scratch in RAM
```

**WAL** (Write-Ahead Logging) is the headline. Without it, readers and writers block each other — every time the analytics CLI runs a query, the indexer waits, and vice versa. With WAL, readers operate against consistent snapshots while the indexer keeps writing.

## 10.2 The schema, annotated

```sql
CREATE TABLE transactions (
    chain           TEXT    NOT NULL,
    tx_hash         TEXT    NOT NULL,
    log_index       INTEGER NOT NULL,        -- NOT redundant — multicall puts many in one tx
    block_number    INTEGER NOT NULL,
    block_timestamp INTEGER NOT NULL,        -- unix seconds

    facilitator     TEXT    NOT NULL,        -- tx.from
    payer           TEXT    NOT NULL,        -- AuthorizationUsed.authorizer / Transfer.from
    recipient       TEXT    NOT NULL,        -- Transfer.to
    amount_raw      TEXT    NOT NULL,        -- u256 as decimal string — full precision
    amount_usdc     REAL    NOT NULL,        -- amount_raw / 1_000_000 — convenience for queries
    nonce           TEXT    NOT NULL,        -- the EIP-3009 auth nonce (bytes32 as hex)

    -- Routing & gas metadata — cheap to capture, expensive to backfill later
    method_selector     TEXT,                -- first 4 bytes of tx.input, hex
    called_contract     TEXT,                -- tx.to
    tx_type             INTEGER,             -- 0=legacy, 1=EIP-2930, 2=EIP-1559
    gas_used            INTEGER,
    effective_gas_price TEXT,                -- u128 as decimal string
    gas_cost_eth        REAL,                -- gas_used * effective_gas_price / 1e18
    base_fee_per_gas    INTEGER,
    tx_nonce            INTEGER,             -- the tx sequence number — NOT the same as `nonce`

    PRIMARY KEY (chain, tx_hash, log_index)
);

CREATE INDEX idx_tx_facilitator  ON transactions(facilitator);
CREATE INDEX idx_tx_payer        ON transactions(payer);
CREATE INDEX idx_tx_recipient    ON transactions(recipient);
CREATE INDEX idx_tx_chain_block  ON transactions(chain, block_number);
CREATE INDEX idx_tx_chain_ts     ON transactions(chain, block_timestamp);
```

**`amount_raw` is TEXT.** SQLite has no native u256. A USDC `amount_raw` fits in u64 (USDC supply is small enough), but the principle generalizes — for tokens with 18 decimals and large supplies, the raw amount is a 78-digit integer. Storing as TEXT preserves full precision; the f64 `amount_usdc` column is for query convenience but should never be summed for accounting purposes (use `amount_raw` and a big-integer library at query time).

**The two nonces.** `nonce` is the EIP-3009 authorization id (32-byte random). `tx_nonce` is the Ethereum transaction sequence number of `tx.from`. Different domain, different size, different type. Naming them distinctly prevents code-review confusion.

**Capture routing metadata at sync time.** `method_selector`, `called_contract`, `tx_type`, `gas_used`, … are essentially free to capture during sync (you already have the parent tx in memory) and extremely expensive to backfill later (would require re-indexing). Capture everything you might plausibly want, even if today's queries don't use it.

**`chain` column.** Single-chain today, multi-chain tomorrow. Cheap insurance, natural prefix on every index.

## 10.3 Batch inserts in one transaction

The single biggest write-perf lever in SQLite:

```python
with conn.transaction():
    for row in batch:
        conn.execute("INSERT OR IGNORE INTO transactions VALUES (...)", row)
    set_cursor(batch.max_block)
```

Without the explicit transaction, every `INSERT` auto-commits with its own fsync. For a 1k-row batch this is 1000× the disk syncs. The transaction wraps the whole batch as a single atomic commit — two orders of magnitude faster *and* gives you "all-or-nothing" semantics for crash recovery.

## 10.4 Idempotent column migrations

SQLite has no `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`. The pattern that works:

```python
existing_columns = {row[1] for row in conn.execute("PRAGMA table_info(transactions)")}
for (name, type) in NEW_COLUMNS:
    if name not in existing_columns:
        conn.execute(f"ALTER TABLE transactions ADD COLUMN {name} {type}")
```

Declarative, re-runnable, no migration tool needed. We grew from 14 to 22 columns over the project's life with this pattern alone.

## 10.5 Schema ownership boundary

In our case, the Rust indexer writes the *ingestion* tables (transactions, facilitators, sync_state, daily_stats, schema_version). The Python analysis layer writes the *analysis* tables (wallet_scores, tx_scores, calibration_labels, …). Both processes share the same SQLite file.

To prevent the two from racing on each other's tables, the boundary is enforced in code:

- The Python side opens the DB read-only when touching ingestion tables: `sqlite_open(path, mode="ro")` and `PRAGMA query_only = ON`.
- The test suite scans every migration file to assert it only targets tables the writing side owns.

Without this guarantee, migration ordering becomes a nightmare and writes collide. With it, you can develop the two processes independently.

---

# Part 11 — Facilitator discovery

## 11.1 There is no on-chain registry

You cannot call a contract on Base and get a canonical list of x402 facilitators. The protocol intentionally has no registry — anyone can run a facilitator, and the social trust is between merchant and facilitator, not enforced on-chain.

The closest thing to a canonical list is the community-maintained directory at **`facilitators.x402.watch`** — 19 facilitators, 94 addresses, spanning Base / Solana / Polygon. Coinbase's own facilitator endpoint (`facilitator.cdp.coinbase.com`) is one entry, with 11 addresses (10 Base + 1 Solana).

The 10 Coinbase Base addresses, for sanity checking:

```
0xdbdf3d8ed80f84c35d01c6c9f9271761bad90ba6
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

## 11.2 Discover from data, anchor against the registry

The right pattern:

1. **Index without filtering on facilitator.** Capture every `tx.from` that has settled an `AuthorizationUsed` event.
2. **Group-by and rank.** The top N by transaction count is your operating facilitator set.
3. **Join against the curated registry** for human-friendly names. Unknown facilitators stay as plain addresses.

The query:

```sql
SELECT facilitator, COUNT(*) AS txs, SUM(amount_usdc) AS volume
FROM transactions
GROUP BY facilitator
ORDER BY txs DESC;
```

Our indexed data shows ~500 distinct `tx.from` addresses settling x402 transactions. Only ~22 of those match curated metadata. The rest are unlabeled. Some are clearly named services running multiple addresses (Coinbase has 10); others are one-offs; some are dispatcher contracts; some may be experimental or short-lived.

**The lesson:** the curated registry is a *naming table*, not a *filter*. If you filter sync on the registry, you blind yourself to every new facilitator the moment they launch.

## 11.3 Using the registry as a coverage check

A useful sanity check at sync time: "did my discovered list include all 10 Coinbase Base addresses?" If no, the filter is wrong — Coinbase is the dominant facilitator by an order of magnitude, so any meaningful sync window should pick all of them up. If a sync that should be complete is missing some Coinbase addresses, that's a smoke alarm — most likely your sighash filter is too narrow (see §6).

---

# Part 12 — Coverage gaps

This is where everything ties together. The current indexer treats "x402" as "EIP-3009 on USDC, classic-signature overload." Each of those qualifiers is a coverage assumption that doesn't hold:

| Qualifier | What we miss when it's wrong |
|---|---|
| "EIP-3009" | The entire `upto` scheme (Permit2) and `batch-settlement` scheme |
| "on USDC" | EURC, and all non-Circle ERC-20 payments via Coinbase's Permit2 path |
| "classic-signature overload" | The bytes-signature overload — probably the majority of x402 traffic by tx count |

The implied total: our indexed numbers are a strict lower bound, probably an order of magnitude below the true total by tx count.

## 12.1 The bytes-sig gap (highest priority)

Per §6 and the findings doc Section 2, our `sighash == 0xe3ee160e`-only filter likely drops most x402 traffic. The fix is straightforward: widen to `{0xe3ee160e, 0xcf092995}`, exclude `{0xef55bec6, 0x88b7ab63}`, re-ingest. Verify against `x402scan.com` or a HyperSync probe before / after.

## 12.2 The Permit2 gap (medium priority, growing)

`upto` and non-USDC ERC-20 traffic both flow through Permit2 (`0x000000000022D473030F116dDEE9F6B43aC78BA3`). Different contract, different event (`SignatureTransfer`), different ABI. Requires a second data path. Same `transactions` schema mostly works if you add a `token_address` column and make `facilitator = tx.from` consistent.

## 12.3 The `batch-settlement` gap (priority unknown)

Spec'd, presumably shipping in some places, no on-chain measurement. Worth probing to size before deciding whether to skip.

## 12.4 The EIP-7702 wrapper gap (lower priority, mostly handled)

Topic-filtered events still catch 7702-routed payments — the inner USDC call still emits `AuthorizationUsed`. The outer sighash filter is what trips. The fix is the same as §12.1: widen the sighash allow-list (or drop the sighash filter and rely on the topic filter alone).

## 12.5 Cross-check anchors

Three external sources are useful for sanity checking your capture:

| Source | What it gives you |
|---|---|
| **`x402scan.com`** (Merit Systems) | Purpose-built x402 block explorer with a SQL-style query API. Most reproducible primary source. |
| **Dune `@hashed_official/x402-analytics`** | SQL aggregations across known facilitators; queries are inspectable on Dune. |
| **`facilitators.x402.watch`** | Community-maintained registry; the anchor set for known facilitators. |

If your captured row count is within ~2× of x402scan / Dune for the same time window, your coverage is roughly complete. If it's order-of-magnitude lower, you're still missing a data path.

---

# Glossary

**ABI (Application Binary Interface)** — The contract that says how function calls and event parameters are encoded into bytes. Solidity generates one for every contract.

**Account (EOA / contract)** — Two address types. EOAs are key-controlled. Contracts hold bytecode and run when called.

**AuthorizationUsed** — The event USDC emits when an EIP-3009 authorization is consumed. Indexer's primary discovery signal for `exact` scheme.

**Backfill** — Catching up the index from chain genesis (or a chosen start block) to current tip. Done once, then live sync takes over.

**Base** — Coinbase's L2 chain on Ethereum. Chain ID 8453, ~2s blocks.

**Block** — An ordered batch of transactions plus a header. Identified by `block_number` (sequential) and `block_hash`.

**Block range** — The `[fromBlock, toBlock]` window passed to `eth_getLogs`. Provider tiers cap this; Alchemy free is ~10 blocks.

**Calldata (a.k.a. `input`)** — The raw bytes of a transaction's payload. For contract calls, conventionally `[selector][args]`.

**Confirmation depth** — How many blocks behind the tip we consider "final." 6 for our live sync, 500 for backfill.

**Cursor** — Persisted last-processed-block marker. Lets resume after a crash without reprocessing or skipping.

**EIP-712** — Standard for structured data signing. EIP-3009 authorizations are EIP-712 messages.

**EIP-1271** — Standard for "is this signature valid for this contract?" Lets smart-contract wallets sign things.

**EIP-1559** — The current transaction pricing model (base fee + priority tip). Type 2 transactions.

**EIP-3009** — Adds `transferWithAuthorization`, `receiveWithAuthorization`, and `cancelAuthorization` to ERC-20. The substrate for x402's `exact` scheme.

**EIP-7702** — Lets an EOA temporarily delegate execution to a contract during one transaction. Pectra upgrade, 2025.

**EOA (Externally Owned Account)** — Key-controlled address. Can initiate transactions.

**ERC-20** — Standard fungible token interface. `balanceOf`, `transfer`, `Transfer` event, etc.

**Event / log** — A piece of data a contract emits during execution. Indexed via topics.

**Facilitator** — The actor that submits x402 settlement transactions on-chain. `tx.from` of the settlement. Pays gas.

**HyperSync** — Envio's chain-archive streaming service. We use it for backfill.

**Idempotent** — Running the same operation twice has the same effect as running it once. Our `INSERT OR IGNORE` makes inserts idempotent.

**`keccak256`** — The hash function used everywhere in EVM. Method selectors and event signature hashes are both `keccak256` outputs.

**L2 (Layer 2)** — A chain that posts data to a parent chain for security. Base is an L2 on Ethereum.

**`log_index`** — The position of a log entry within its transaction's receipt. Zero-indexed. Critical for multicall pairing.

**Method selector** — First 4 bytes of `keccak256` of the function signature. What you filter on for "which function was called."

**Multicall (Multicall3)** — A contract that lets one transaction batch many sub-calls. Address `0xcA11bde05977b3631167028862bE2a173976CA11`.

**Nonce (two meanings)** — (1) The EIP-3009 `bytes32` authorization id. (2) The transaction sequence number of the sender EOA. We store both, named `nonce` and `tx_nonce`.

**N+1 problem** — Doing N follow-up queries (one per result) when one batched query would do. Naïve event-based indexing falls into this; block-batching avoids it.

**Permit2** — Uniswap's universal ERC-20 permission contract at `0x000000000022D473030F116dDEE9F6B43aC78BA3`. Backs x402's `upto` scheme and Coinbase facilitator's non-USDC ERC-20 path.

**Proxy / implementation** — Pattern for upgradeable contracts. The proxy's address is stable; the implementation it forwards to can change.

**Reorg** — Block reorganization. A formerly-canonical block gets replaced. Defended against by confirmation depth.

**RPC (JSON-RPC)** — The wire protocol for talking to Ethereum nodes. `eth_getLogs`, `eth_getBlockByNumber`, etc.

**Sighash** — Slang for "method selector." First 4 bytes of `keccak256(function_signature)`.

**`tx.from`** — The EOA that signed and submitted the transaction. For x402 settlements, this is the facilitator.

**`tx.to`** — The address being called. For direct USDC calls, the USDC proxy. For wrapped calls, a router or Multicall3.

**Topic** — A 32-byte indexed field in a log entry. `topics[0]` is the event signature hash; `topics[1..]` are indexed parameters. You can filter on topics server-side via `eth_getLogs`.

**Transaction hash (`tx_hash`)** — `keccak256` of the signed transaction payload. Canonical identifier.

**USDC** — Circle's USD-pegged stablecoin. On Base at `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` (proxy). 6 decimals.

**WAL (Write-Ahead Log)** — SQLite mode that lets concurrent readers operate without blocking the writer. Essential for the indexer + analytics pattern.

**x402** — Coinbase's HTTP payment protocol over on-chain stablecoin settlement. Three schemes: `exact`, `upto`, `batch-settlement`.

**`x402scan.com`** — Community-built block explorer dedicated to x402. Use as a coverage anchor.

---

# Reading order from here

Now you should be able to read the findings doc front-to-back without needing to look anything up. The chapter map:

- Findings §1 ("The domain in 60 seconds") → corresponds to Parts 3 and 4 here.
- Findings §2 ("The filtering trap") → expansion of Part 6.
- Findings §3 ("The join trap") → expansion of §5.2.
- Findings §4 ("Companion-Transfer pairing") → expansion of Part 7.
- Findings §5 ("Two data paths") → expansion of Part 8.
- Findings §6 ("Reorgs & cursors") → expansion of Part 9.
- Findings §7 ("Storage") → expansion of Part 10.
- Findings §8 ("Domain-model pitfalls") → mostly things called out in Parts 3 and 10.
- Findings §9 ("Facilitator registry") → expansion of Part 11.
- Findings §10 ("Operational pattern catalogue") → workflow tips; light on new concepts.
- Findings §11 ("What I'd do differently") → reflections.
- Findings §12 ("Coverage gaps") → expansion of Part 12.
- Findings §13 ("What's still open") → research backlog.
- Findings Appendix A → reference card for selectors, addresses, constants. Use the glossary above for the *meaning*, Appendix A for the *values*.
- Findings Appendix B → empirical numbers from our indexed dataset, with explicit coverage caveats.
