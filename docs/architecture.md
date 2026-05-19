# Fathom — Technical Architecture (v1)

**Version:** v0.1
**Status:** Approved (sits alongside `project-spec.md` and `what-to-build.md`)
**Scope:** v1 only. v2/v3 protocols (L402/Lightning, others) are anticipated but not architected here.

---

## 1. Overview

Fathom is a weekly published index of agent-mediated payment flow. The system has three layers, runs locally on the operator's machine for v1, and is implemented in Go against a single Postgres instance. Everything is plumbing in service of one weekly artifact.

The locked technology surface:

- **Go** — every long-running process and every one-shot tool
- **Postgres** — single instance, sole source of truth
- **Docker Compose** — orchestrates Postgres + collectors + init-db locally; the supervisor for v1
- **scheduling** — TBD; the daily probe and weekly publisher will land with either an in-binary scheduler, a sidecar cron container, or host crontab calling `docker compose run`
- **git** — distribution channel for the public data repo and weekly artifacts

No Kubernetes, no Kafka, no managed event store, no Clickhouse, no web frontend, no VPS for v1. These are not merely avoided — their absence is a load-bearing design choice the rest of the architecture depends on.

End-to-end pipeline:

```
                ┌─────────────────────────────────────────────────┐
                │                LAYER 1: COLLECTORS              │
                │                                                 │
   Base RPC ───►│  base-collector  ─┐                             │
                │                   │                             │
 Solana RPC ───►│  solana-collector ├──► inserts ─────────────┐   │
                │                   │                         │   │
 HTTP endpoints►│  probe-collector ─┘                         │   │
                └─────────────────────────────────────────────┼───┘
                                                              │
                ┌─────────────────────────────────────────────▼───┐
                │                LAYER 2: WAREHOUSE               │
                │                                                 │
                │  raw tables:  payments  services  probe_results │
                │                                                 │
                │  versioned views:  weekly_payment_volume_v1     │
                │                    weekly_active_services_v1    │
                │                    weekly_new_services_v1       │
                └────────────────────────────────────┬────────────┘
                                                     │ SELECT
                ┌────────────────────────────────────▼────────────┐
                │                LAYER 3: PUBLISHER               │
                │                                                 │
                │  read views → emit JSON / CSV / Markdown        │
                │             → render charts                     │
                │             → write draft social post           │
                └──────────┬───────────────────────────┬──────────┘
                           │ git push                  │ writes file
                           ▼                           ▼
                  public data repo             drafts/ (human review)
```

Each layer owns exactly one boundary. The interfaces between them — listed in §5 — are the most consequential decisions in the design.

---

## 2. Layer 1 — Collectors

**Responsibility:** observe the world and record what they see. Nothing else. Collectors do not classify, normalize across sources, or compute. They write *facts as observed*, and downstream layers decide what those facts mean.

### Components

Three runnable units, one per data source. Each is its own Go binary and its own systemd service:

- **`base-collector`** — connects to a Base RPC endpoint, subscribes to (or polls) the known x402 facilitator contracts and settlement addresses, decodes payment events, inserts into `payments`.
- **`solana-collector`** — same shape against Solana RPC. Different decoding logic, identical output table.
- **`probe-collector`** — reads a hand-curated YAML registry of 402-speaking HTTP endpoints, probes each on a daily schedule, appends to `probe_results`, upserts `services`.

### The shared collector contract

Every collector follows the same five rules. Treating them as a contract is what keeps the layer operable in 30 minutes a week:

1. **Cursor-held.** The collector owns its own cursor (`from_block`, `since_timestamp`). Nothing else reads or writes it.
2. **Idempotent writes.** Re-running over the same range produces no duplicates. Achieved via natural keys (e.g. `(chain, tx_hash, log_index)` for payments).
3. **Independent failure.** If `solana-collector` crashes, the other two continue. There is no shared in-process state between collectors.
4. **Replayable.** Every collector accepts `--from-block X` / `--since T` and can be re-run against historical ranges without manual cleanup.
5. **Append-mostly.** Writes are inserts or idempotent upserts. Mutation of existing rows is reserved for narrow, documented cases.

### Inputs and outputs

- **Input:** RPC subscriptions (Base, Solana) and HTTP requests (probe).
- **Output:** rows in `payments`, `services`, `probe_results`. No file artifacts, no external API calls, no logs of record.

### What the layer does NOT do

- No classification of payments as "agent" vs "non-agent" — every observed x402 payment is recorded; the methodology decides what counts.
- No USD normalization beyond recording the asset and a contemporaneous USD reference per row. The conversion *rule* lives in views.
- No cross-source joins. Collectors never read each other's rows.

---

## 3. Layer 2 — Warehouse

**Responsibility:** be the source of truth for what was observed, *and* the source of truth for how observations become metrics. Two roles, deliberately separated inside the same Postgres database.

### Raw tables (the facts)

Three tables, append-mostly, never the subject of methodology debates:

- **`payments`** — one row per observed payment. Columns include `observed_at`, `source`, `chain`, `protocol`, payer/payee addresses, `payee_service_id` (nullable join to `services`), `amount`, `asset`, `asset_usd_at_time`, `raw_tx_ref`.
- **`services`** — one row per known 402-speaking service. URL, first seen, last probed, status, declared methods, price observations, category, notes.
- **`probe_results`** — append-only history of every probe of every service. The time-series of the supply side.

The shape of these tables is allowed to evolve (new columns added), but their *meaning* is preserved. A `payments` row from week 1 must still be queryable identically in week 52.

### Versioned views (the methodology)

Metrics live in SQL views, never in the raw tables and never computed inside the publisher:

- `weekly_payment_volume_v1`
- `weekly_active_services_v1`
- `weekly_new_services_v1`

The versioning rule is an architectural invariant, not a guideline:

> **A methodology change creates a new view (`_v2`). The old view is preserved and remains runnable indefinitely.**

This is what makes the index defensible. Anyone asking "what did your numbers say in March" runs the v1 views; anyone asking "what's the current methodology" runs the v_N views. Both answers are reproducible from the same raw tables, forever.

### Inputs and outputs

- **Input:** writes from the three collectors.
- **Output:** rows returned by view `SELECT`s. Nothing else.

### What the layer does NOT do

- No background jobs, no triggers, no materialized views in v1. The views are plain SQL views; the publisher pays the query cost once a week, which is fine at v1 scale.
- No table-level methodology. Anything that could be called a *decision* lives in a view definition.

---

## 4. Layer 3 — Publisher

**Responsibility:** turn warehouse state into the weekly artifact. Runs once a week from cron. Single Go binary, deliberately boring.

### Sub-components (all inside one binary)

- **View reader** — `SELECT`s the current week's results from each `weekly_*` view.
- **Artifact emitter** — produces three files per week: a JSON snapshot, a CSV, and a Markdown summary.
- **Chart renderer** — one chart per metric, consistent visual style. Go-native plotting if it suffices; small Python shell-out as fallback (decision deferred to week 1, see §6).
- **Draft writer** — writes the social post (text + chart images) into a local `drafts/` folder for human review. **Never posts to X.**
- **Repo pusher** — commits the three artifact files to the public data repo with a tagged release (`week-2026-W21` style).

### Inputs and outputs

- **Input:** view results from Layer 2.
- **Output:** a git commit + tag in the public data repo; local files in `drafts/`.

### The human-in-the-loop gate

The lack of an auto-post code path is architectural, not policy. There is no function in this binary that calls the X API. To publish, a human reads the `drafts/` folder and posts manually. This property is intended to hold for the first six months minimum and is the most important non-feature in the system.

---

## 5. Interfaces between layers

Three contracts hold the architecture together. They are stated as rules because their enforcement is what makes each layer replaceable in isolation.

**Collectors → Warehouse.**
Collectors write to raw tables only. They never read each other's rows, and they never read views. Cursors are owned by the writing collector — nothing else touches them. This is what makes "one collector crashed" a non-event for the rest of the system.

**Warehouse → Publisher.**
The publisher reads views, never raw tables. This is the rule that makes the methodology auditable: every number in the weekly artifact traces to exactly one view definition, and that definition is frozen. If the publisher were allowed to compute a metric in Go, that computation would be invisible to the methodology audit — so it isn't allowed.

**Publisher → External.**
The publisher's only outputs are (a) git commits to the public data repo and (b) files in a local `drafts/` folder. There is no other egress path — no HTTP client for X, no SMTP, no webhook. The "human in the loop" property is enforced by the absence of code, not by a flag.

Each cut also doubles as a test boundary: a collector is tested by inserting rows and inspecting raw tables; a view is tested by inserting raw rows and inspecting view output; the publisher is tested by stubbing views and inspecting emitted files.

---

## 6. Open architectural decisions

Status at 2026-05-20:

- **Repo layout.** ✅ Decided: monorepo for code (this repo). The public data repo will be created when the publisher first ships and pushes weekly artifacts to it.
- **Process supervisor.** ✅ Decided for v1: Docker Compose orchestrates Postgres + the three collectors + a one-shot `init-db` container locally. systemd is deferred until v1 grows a deployment target beyond the operator's laptop.
- **Project name.** ✅ Decided: Fathom (kept).
- **Chart pipeline.** Still open. Go-native (`gonum/plot` or similar) or shell out to a minimal Python script? Decision driven by whether Go-native output meets the visual-identity bar in the first publish week.
- **Scheduling (probe daily, publisher weekly).** Open. Three viable paths — internal Go scheduler with `--once` vs daemon mode, a cron sidecar container (e.g., `ofelia`), or host crontab calling `docker compose run`. Lock with the first binary that needs it.

The project setup that implements the closed items above is specified in [`superpowers/specs/2026-05-19-project-setup-design.md`](./superpowers/specs/2026-05-19-project-setup-design.md).

---

*This document covers architecture only. Tech stack rationale, methodology versioning detail, failure modes, ops, and security are out of scope here and belong in (or will belong in) sibling documents.*
