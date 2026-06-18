# Fathom

A reproducible measurement layer for agent-mediated payments: it indexes the observable x402 settlement surface (Base), classifies every payment by identity, and publishes the result as a static, citable dashboard. Architecture details: [`docs/architecture.md`](docs/architecture.md).

## Prerequisites

macOS with Homebrew. The setup expects:

- `go` (1.26+)
- `just`
- `lefthook`
- `docker` + `docker compose`

Install missing tools:

```sh
brew install go just lefthook
```

Docker is installed via Docker Desktop.

## First-time setup

```sh
cp .env.example .env
just hooks       # install lefthook git hooks — required, no auto-install on clone
just up          # start postgres + run migrations/views + start collector services
```

## Daily commands

```sh
just              # list all recipes
just fmt          # gofumpt + goimports
just lint         # golangci-lint
just test                # go test -race (unit)
just test-integration    # go test -tags=integration (real postgres via testcontainers)
just dev <binary>        # run a binary from the host with APP_ENV=local
just build        # produce all four binaries into ./bin/
just vuln         # govulncheck

just up           # bring the stack up
just init-db      # re-run migrations + views (idempotent)
just migrate up   # apply pending migrations from host
just apply-views  # re-apply view definitions
just psql         # psql shell against the compose postgres
just logs         # tail logs (all services); pass a service name to scope
just run <bin>    # one-shot invocation: `just run publisher`
just nuke         # tear down and drop the postgres volume
```

## Layout

```
cmd/<binary>/main.go        # one entrypoint per long-running unit
config/<binary>/            # per-binary TOML (base.toml, local.toml, etc.)
internal/                   # shared packages (config, log, db)
database/migrations/        # goose .sql migrations — NNNNN_name.sql
database/views/             # versioned SQL views — methodology lives here
database/init/init-db.sh    # one-shot: goose up + apply views (inside init-db container)
database/testdata/          # seed scripts (empty in v1)
docs/                       # spec, architecture, plans
```

The split between `database/migrations/` (tables) and `database/views/` (methodology) is load-bearing — see the architecture doc.

## Dashboard data (publisher)

The dashboard is served from precomputed static JSON — the DB is never queried at
request time. Regenerate after a backfill:

    go run ./cmd/publisher/ rollup            # rebuild all metrics tables (cube, medians,
                                              #   price points, gas, velocity) in one tx
    go run ./cmd/publisher/ emit --out dist   # write dist/*.json

Rollup is the heavy step — budget ~2h per 20M rows on Docker-for-Mac (the windowed
percentile sorts dominate); emit reads only the small tables and takes ~1s, so
re-emitting after a claims-file edit is free.

Emit also writes the dashboard page itself (`index.html` + assets, embedded in the
publisher binary from `web/site/`) into the same directory — Caddy serves one
self-contained folder. Edit the page under `web/site/`, rebuild, re-emit.
Preview locally: `python3 -m http.server 8901 -d dist` → http://localhost:8901
(check the status bar shows `conservation ✓` and the stamps).

Curated inputs (committed, git-reviewed):

- `data/eth-usd-monthly.json` — monthly ETH/USD reference prices (gas → USD);
  rollup fails if a month present in `payments` has no price.
- `data/claims.json` — the claimed-vs-measured ledger on the economy page;
  emit resolves each claim's `measured_metric` against the cube.

`metrics_daily_v1` is the rollup cube (`day × chain × facilitator × attribution ×
amount_band`); `metrics_window_stats_v1`, `metrics_price_points_v1`,
`metrics_gas_daily_v1`, `metrics_velocity_daily_v1` carry the non-mergeable economy
stats. Artifacts are stamped with `methodology_version` and the latest data day.
See `docs/superpowers/specs/2026-06-11-economy-page-data-design.md`.

## Anatomy (transaction dossier graph)

A standalone internal web app: paste a transaction hash, get an interactive
graph of that one transaction — principals, payment events, the on-chain call,
and per-address stats — read from `payment_x402_v1`.

```bash
just anatomy-web        # build the React/Vite frontend into web/anatomy/dist
just anatomy            # run the service (API + UI) on http://localhost:8090
```

Dev loop with hot reload: run `just anatomy` (API on :8090) and, in another
terminal, `cd web/anatomy && npm run dev` (Vite on :5173, proxies /api).

Local-only in v1. Identity / on-chain-RPC / internet enrichment are designed-in
but ship as disabled stubs.

## Production deployment

Production runs on a single Ubuntu VPS via `docker-compose.prod.yml` (GHCR images,
tuned Postgres, Caddy). Merges to `main` push images to GHCR; deploys are a manual
`./deploy.sh` on the box. Full runbook: [`DEPLOYMENT.md`](DEPLOYMENT.md).

## Conventions

- **Commits:** `<type>: <description>` where type is one of `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`.
- **Pre-commit hooks:** lefthook runs gofumpt, goimports, golangci-lint, and `go test ./...` on every commit touching `*.go`. To install: `just hooks`. To bypass in an emergency: `LEFTHOOK=0 git commit ...`.
- **Methodology versioning:** new view ⇒ new file (`weekly_payment_volume_v2.sql`). Old view files are never modified or deleted.
