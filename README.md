# Fathom

A weekly published index of agent-mediated payment flow. Architecture details: [`docs/architecture.md`](docs/architecture.md).

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
just test         # go test -race
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
cmd/<binary>/main.go   # one entrypoint per long-running unit
internal/              # shared packages (emerge with the first real collector)
migrations/            # goose .sql migrations — NNNNN_name.sql
views/                 # versioned SQL views — methodology lives here
scripts/init-db.sh     # one-shot: goose up + apply views (used inside init-db container)
docs/                  # spec, architecture, plans
```

The split between `migrations/` (tables) and `views/` (methodology) is load-bearing — see the architecture doc.

## Conventions

- **Commits:** `<type>: <description>` where type is one of `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`.
- **Pre-commit hooks:** lefthook runs gofumpt, goimports, golangci-lint, and `go test ./...` on every commit touching `*.go`. To install: `just hooks`. To bypass in an emergency: `LEFTHOOK=0 git commit ...`.
- **Methodology versioning:** new view ⇒ new file (`weekly_payment_volume_v2.sql`). Old view files are never modified or deleted.
