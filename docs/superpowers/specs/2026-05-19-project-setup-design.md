# Project Setup — Design

**Date:** 2026-05-19
**Status:** Approved
**Scope:** Initial tooling, layout, and dev environment for the Fathom Go monorepo. No business logic. Sits beneath `docs/architecture.md` — implements its working assumptions about repo layout.

---

## 1. Goal

Make the repo ready for the first line of business code: any contributor (one, for now) can clone, run one command, and have a working dev environment with linting, formatting, tests, migrations, and the four binaries from the architecture buildable in Docker.

Nothing in this setup commits to a deployment target. The architecture doc's systemd/cron/VPS plan is deferred; local Docker is the v1 dev environment.

## 2. Module and layout

- **Module path:** `github.com/lukostrobl/fathom`
- **Go version:** latest stable (pin in `go.mod`)
- **Tree:**

```
fathom/
├── cmd/
│   ├── base-collector/main.go
│   ├── solana-collector/main.go
│   ├── probe-collector/main.go
│   └── publisher/main.go
├── internal/
│   ├── config/        # env + flag parsing, shared
│   ├── db/            # pgx wiring, migration entrypoint
│   └── collectors/    # shared collector primitives (cursor, idempotent upsert helpers)
├── migrations/        # goose .sql files; one numbered file per change
├── views/             # raw SQL view definitions, versioned (weekly_payment_volume_v1.sql, ...)
├── docs/
├── .github/workflows/ci.yml
├── Dockerfile
├── docker-compose.yml
├── compose.override.yml.example
├── .dockerignore
├── .editorconfig
├── .gitignore
├── .golangci.yml
├── lefthook.yml
├── justfile
└── go.mod
```

The split between `migrations/` (table changes) and `views/` (versioned methodology) matches the architecture's two-role warehouse: raw tables evolve via migrations, methodology evolves by adding new `_vN` view files alongside the old ones, never replacing them.

## 3. Formatting

- **`gofumpt`** — stricter superset of `gofmt`. Run via lefthook on pre-commit. No config; the rules are the rules.
- **`goimports`** — group local imports separately from stdlib/third-party, ordered by module prefix.

Both are pinned via `tool` directives in `go.mod` (Go 1.24+ mechanism) so `go tool gofumpt` and `go tool goimports` always run a known version.

## 4. Linting

- **`golangci-lint`** with `.golangci.yml`. Enabled linters:
  - `errcheck` — every error checked
  - `govet`, `staticcheck`, `gosimple` — correctness
  - `ineffassign`, `unused` — dead code
  - `gosec` — basic security smell (SQL string concat, weak crypto)
  - `revive` — style, replaces deprecated `golint`
  - `gocritic` — diagnostic + style suggestions

  Disabled by default: `gofumpt`/`gci` (we run gofumpt directly), `lll`, `funlen`, `gocyclo` (style noise, not bug-catching).

- Pre-commit runs `golangci-lint --new-from-rev=HEAD` for speed; CI runs full sweep.

## 5. Pre-commit hooks

- **`lefthook`** — single Go binary, no Python dependency. `lefthook.yml` config.
- **`pre-commit` stage:**
  - `gofumpt -l -w` on staged Go files
  - `goimports -w` on staged Go files
  - `golangci-lint run --new-from-rev=HEAD`
  - `go test ./...` (only if Go files staged)
- **`pre-push` stage:**
  - `go mod tidy` diff check (fail if mod/sum would change)

Skippable via `LEFTHOOK=0` for emergencies.

## 6. Task runner

- **`justfile`** — `just` recipes:
  - `just fmt` — gofumpt + goimports
  - `just lint` — golangci-lint run
  - `just test` — go test ./...
  - `just build` — build all four binaries
  - `just up` — docker compose up -d
  - `just down` — docker compose down
  - `just migrate [up|down|status]` — goose against the compose Postgres
  - `just apply-views` — apply the SQL files in `views/` to Postgres (idempotent: `CREATE OR REPLACE VIEW`)
  - `just psql` — psql shell into the compose Postgres
  - `just run <binary>` — `docker compose run --rm <binary>` (for one-shot/cron-style invocations)

## 7. Database migrations

- **`goose`** — embeddable (so a binary can run migrations on startup if needed) and a usable CLI.
- Migrations live in `migrations/` as `NNNN_description.sql` with `-- +goose Up` / `-- +goose Down` markers.
- Views are NOT in `migrations/`. They live in `views/` as separate files and are applied via `just apply-views`. Rationale: a view definition change is methodology, not schema, and creates a new `_vN` file rather than altering anything in place — different lifecycle than tables.

## 8. Docker

- **Single multi-stage `Dockerfile`** with `ARG BINARY`:
  - Builder stage: `golang:<pinned>` → `go build -o /out/app ./cmd/${BINARY}`
  - Runtime stage: distroless or `alpine` (decision deferred to first build) with the static binary
  - One image per binary, tagged `fathom-<binary>`

- **`docker-compose.yml`:**
  - `postgres` service (pinned version, named volume)
  - `init-db` service — one-shot: runs `goose up` for tables, then applies all `views/*.sql` (idempotent `CREATE OR REPLACE VIEW`), then exits. Other services `depends_on: init-db` with `condition: service_completed_successfully`. Keeps "fresh clone → `just up` → working db" as a one-command path.
  - `base-collector`, `solana-collector`, `probe-collector` — `restart: unless-stopped`
  - `publisher` — `restart: "no"`, intended for `docker compose run --rm publisher` invocations
  - Env via `.env` (gitignored); a `.env.example` is committed

- **`compose.override.yml.example`** — shows how to mount source for live rebuild, bind-mount a host RPC config, etc. Copy to `compose.override.yml` and edit; the override is gitignored.

## 9. CI

- **GitHub Actions**, single workflow `ci.yml`:
  - Trigger: PR + push to main
  - Jobs (parallel):
    - **lint:** `golangci-lint` full sweep
    - **test:** `go test -race ./...` against a Postgres service container
    - **vuln:** `govulncheck ./...`
    - **build:** `docker build` each binary (matrix on BINARY arg) to catch Dockerfile drift
- No release workflow yet — adding one is YAGNI until we ship beyond local.

## 10. Deferred (explicitly not built in this setup)

- **Scheduling for probe-collector (daily) and publisher (weekly).** Three viable paths — internal Go scheduler, `ofelia` cron container, host crontab calling `docker compose run`. Decision lives with the first binary that needs it. Binaries will be one-shot-capable so any path works later.
- **systemd unit files.** The arch doc's working assumption; revisit if/when we move off the local Docker setup.
- **Deployment / publisher's git push to public data repo.** Setup will wire the binary's existence; the actual credentials and push target are configured at publish time, not now.
- **Mock generators (`mockgen`, `moq`).** Add when a test actually demands a mock.
- **Release tooling (`goreleaser`).** Not needed for a local-only dev environment.
- **Conventional-commit enforcement.** Overkill for a solo project.

## 11. Success criteria

After implementation, the following must all be true on a fresh clone:

1. `just up` brings up Postgres, runs `init-db` (migrations + views) to completion, and leaves the three long-running collector services healthy.
2. `just lint && just test` exit 0 with the placeholder binaries.
3. `just build` produces four binaries.
4. `git commit` triggers gofumpt/goimports/golangci-lint/test via lefthook.
5. Pushing to a PR runs the CI workflow green.

No business logic is required to satisfy these. The binaries can be `func main() { log.Println("base-collector starting") }` stubs at the end of this setup.
