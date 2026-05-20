# Golang Reference Guide Alignment — Design

**Date:** 2026-05-20
**Status:** Drafted, awaiting user review
**Scope:** Align the fathom foundation with `../golang-ref-guide/` while honoring v1's tight scope and CLAUDE.md's simplicity rule. Builds on `2026-05-19-project-setup-design.md` — does not replace it.

---

## 1. Goal

Adopt the golang-ref-guide posture for fathom by:

1. Building a **vertical slice** through `base-collector` that exercises every new pattern end-to-end (config loading, slog, pgx, testcontainers), then
2. Replicating that pattern to `solana-collector`, `probe-collector`, and `publisher`, and
3. Recording reference patterns that don't yet have a consumer (REST/chi, OTEL, money, uuid, repository) in `docs/conventions.md` so the choices are locked even when the code isn't.

The result is foundation, not feature: collectors stay stubs; what changes is how they're wired.

## 2. Decisions locked in brainstorming

| Question | Decision |
|---|---|
| How much of the reference to adopt | Wholesale, with simplicity-driven deferrals |
| Parts of the reference that don't yet apply (chi, REST, OTEL) | Document in `docs/conventions.md`; don't scaffold code |
| Migration tool | Keep **goose** (already wired). Documented deviation from reference's `golang-migrate` |
| Top-level DB layout | Adopt **`/database/{migrations,views,init,testdata}`** |
| Phasing | **Vertical slice first** through `base-collector`, then replicate |
| Compose: configs in image vs mounted | **Bake into image** (configs are code). Mount only `local.secrets.toml` for dev |
| `APP_ENV` source | **Environment variable** (matches koanf reference) |

## 3. Adoption matrix

| Reference recommendation | Action |
|---|---|
| `/cmd/`, `/internal/`, `/config/<binary>/`, `/database/{migrations,views,init,testdata}` | ✅ Adopt now |
| `knadh/koanf` + layered TOML + generic `ParseConfig[T BasicConfigurator]` | ✅ Adopt now |
| `log/slog` | ✅ Adopt now |
| `jackc/pgx/v5` + `pgxpool` | ✅ Adopt now |
| `testcontainers-go` (one integration test on the slice) | ✅ Adopt now |
| `google/uuid` | ❌ Defer — no consumer in stubs |
| `cockroachdb/apd/v3` | ❌ Defer — no payment row to hold money yet |
| `sqlc` | ❌ Defer — raw pgx fine for v1 inserts |
| `golang-migrate` | ❌ Keep `goose` (deviation documented in `docs/conventions.md`) |
| `git-cliff` | ❌ Defer until first release |
| `go-chi/chi`, REST patterns, OTEL tracing, DDD/CQRS/repository | 📄 `docs/conventions.md` only |
| Multi-stage Docker, golangci-lint v2, gosec, gofumpt, govulncheck, lefthook, justfile | ✅ Already in place; keep |

Rule: a library lands in `go.mod` when it has a real consumer in committed code. No speculative wrappers.

## 4. Target directory layout

```
fathom/
├── cmd/
│   ├── base-collector/main.go        # rewritten by the slice
│   ├── solana-collector/main.go      # stub until replication
│   ├── probe-collector/main.go       # stub until replication
│   └── publisher/main.go             # stub until replication
├── config/
│   └── base-collector/               # added with the slice
│       ├── base.toml                 # checked in, defaults
│       ├── local.toml                # checked in, dev overrides
│       └── local.secrets.toml        # gitignored
├── database/                         # NEW container for SQL
│   ├── migrations/                   # moved from /migrations
│   │   └── 00001_init.sql
│   ├── views/                        # moved from /views
│   │   └── _smoke_v1.sql
│   ├── init/
│   │   └── init-db.sh                # moved from /scripts
│   └── testdata/                     # empty for now
├── internal/
│   ├── config/                       # ParseConfig[T] + BasicConfig
│   │   ├── config.go
│   │   ├── errors.go
│   │   └── config_test.go
│   ├── log/
│   │   └── log.go                    # slog setup from BasicConfig
│   └── db/
│       └── db.go                     # pgxpool init from cfg.Database.URL
├── docs/
│   ├── architecture.md               # paths updated for /database/
│   ├── conventions.md                # NEW: doc-only patterns
│   ├── project-spec.md
│   ├── what-to-build.md
│   └── superpowers/...
├── Dockerfile                        # COPY paths updated for /database/init/
├── docker-compose.yml                # volume mounts updated; APP_ENV added
├── justfile                          # paths updated; `just dev <bin>` added
├── lefthook.yml
├── .golangci.yml
├── .env.example                      # APP_ENV=local added
└── go.mod
```

## 5. The vertical slice (base-collector)

### 5.1 Config files

**`config/base-collector/base.toml`:**

```toml
name = "base-collector"
version = "v0"
with_debug_profiler = false

[log]
is_pretty = false
level = "info"
```

**`config/base-collector/local.toml`:**

```toml
env = "local"
secrets_path = "config/base-collector/local.secrets.toml"

[log]
is_pretty = true
level = "debug"
```

`database.url` is loaded from the `DATABASE__URL` environment variable (koanf maps `FOO__BAR` → `foo.bar`). Compose already sets `DB_URL`; we add `DATABASE__URL=${DB_URL}` to the collector's env block.

`local.secrets.toml` is gitignored; the path in `local.toml` is loaded only if the file exists.

### 5.2 `internal/config/config.go`

Lifted from `golang-ref-guide/configuration.md` with two deletions:

- Drop the `Observability` sub-struct (no OTEL collector in v1).
- Drop the `HTTP` sub-struct (no HTTP server in v1).

Result — `BasicConfig` holds: `Env`, `Name`, `Version`, `SecretsPath`, `Log{IsPretty, Level}`, `WithDebugProfiler`. Plus the `BasicConfigurator` interface and the generic `ParseConfig[Config BasicConfigurator](binaryName, environment string)` running the four layers (base.toml → `<env>.toml` → ENV → secrets file) and the startup validation that errors with `MissingRequiredFieldsError` if `Env`, `Name`, or `Version` is empty.

`MissingRequiredFieldsError` lives in `internal/config/errors.go`.

### 5.3 `internal/log/log.go`

```
func New(b config.BasicConfig) *slog.Logger
```

Returns `slog.New(slog.NewTextHandler(...))` if `b.Log.IsPretty`, else `slog.New(slog.NewJSONHandler(...))`. Level parsed from `b.Log.Level` (`debug`/`info`/`warn`/`error`). Calls `slog.SetDefault(logger)` so package-level helpers work.

### 5.4 `internal/db/db.go`

```
func Open(ctx context.Context, url string) (*pgxpool.Pool, error)
```

Calls `pgxpool.New(ctx, url)`; pings; wraps errors with `fmt.Errorf("open db pool: %w", err)`. Caller owns `defer pool.Close()`.

### 5.5 `cmd/base-collector/main.go`

```go
type Config struct {
    config.BasicConfig
    Database struct {
        URL string `koanf:"url"`
    } `koanf:"database"`
}

func (c Config) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

func main() {
    env := os.Getenv("APP_ENV")
    if env == "" {
        env = "local"
    }
    cfg, err := config.ParseConfig[Config]("base-collector", env)
    if err != nil {
        slog.Error("parse config", "err", err)
        os.Exit(1)
    }
    logger := applog.New(cfg.BasicConfig)
    ctx := context.Background()
    pool, err := db.Open(ctx, cfg.Database.URL)
    if err != nil {
        logger.Error("open db", "err", err)
        os.Exit(1)
    }
    defer pool.Close()
    logger.Info("base-collector ready", "version", cfg.Version)
}
```

Exits cleanly after logging ready. The real collector loop is a separate, later task.

### 5.6 Tests

**`internal/config/config_test.go`** — table-driven unit tests using temp dirs:

- Missing `base.toml` → returns an error.
- Missing `<env>.toml` → returns an error.
- ENV var `LOG__LEVEL=warn` overrides TOML.
- Missing `Name`/`Version`/`Env` → `MissingRequiredFieldsError`.
- Secrets file present → overlays correctly; absent → silently skipped.

**`cmd/base-collector/integration_test.go`** (build tag `//go:build integration`):

- Uses `testcontainers-go/modules/postgres` to bring up a Postgres container.
- Runs goose programmatically against the embedded migrations to apply schema.
- Calls `db.Open` with the container's connection string.
- Asserts `SELECT 1` succeeds and the `_fathom_meta` row from migration `00001_init` is present.

Tag-gated so `just test` stays fast; `just test-integration` runs the slow path.

## 6. Files moved (mechanical)

| From | To |
|---|---|
| `migrations/` | `database/migrations/` |
| `views/` | `database/views/` |
| `scripts/init-db.sh` | `database/init/init-db.sh` |

Call sites updated:

- `docker-compose.yml` — `init-db` volumes change to `./database/migrations` and `./database/views`.
- `Dockerfile` — `COPY scripts/init-db.sh /init-db.sh` → `COPY database/init/init-db.sh /init-db.sh`.
- `database/init/init-db.sh` — internal paths if any (currently the script uses `/migrations` and `/views` mount points; those stay the same since they're mount destinations).
- `justfile` — `migrate` recipe uses `-dir database/migrations`; `apply-views` glob uses `database/views/*.sql`; new `test-integration` recipe runs `go test -tags=integration -race ./...`; new `dev <binary>` recipe runs a binary from the host with `APP_ENV=local` for outside-of-compose iteration.
- `README.md` — Layout section updated.
- `docs/architecture.md` — any path references updated; the architecture itself is unchanged.

## 7. Compose changes

- Bake configs into the image: the builder stage already runs `COPY . .` so `config/` is present there. The runtime stage currently only copies the compiled binary; add `COPY --from=builder /src/config /config` to the runtime stage so the binary can find `config/<binary>/base.toml` at runtime. Set the container `WORKDIR /` (already the default for distroless) so the relative path `config/<binary>/base.toml` from `ParseConfig` resolves.
- Per-collector env: add `APP_ENV: local` and `DATABASE__URL: ${DB_URL}` to each collector service's `environment:` block.
- Local dev secrets mount: optional `./config/base-collector/local.secrets.toml:/config/base-collector/local.secrets.toml:ro` mount, guarded by `compose.override.yml`. Not required for the slice to work.

## 8. `docs/conventions.md` (doc-only patterns)

One file, short. Each section is a paragraph plus a link to the relevant `../golang-ref-guide/<file>.md`.

- **REST API.** `net/http` server with timeouts and graceful shutdown; `go-chi/chi` router (not context-based); URL versioning + date header; net/http-compatible middleware; dep-injected handlers.
- **Errors.** Typed `marshalError` consts for simple cases; struct errors for context-bearing cases; `errors.Is` / `errors.AsType` (Go 1.26); always wrap with `%w`; exported errors are typed.
- **OTEL tracing.** `opentelemetry-go` SDK; `otelhttp` outbound; `otelchi` middleware; collector address from a future `BasicConfig.Observability` block.
- **Money.** `cockroachdb/apd/v3` with explicit `apd.Context.WithPrecision(20)`; pgx `Numeric` ↔ `*apd.Decimal`; JSON-as-string wrapper for API responses.
- **UUID.** `google/uuid`. Never `satori/go.uuid`.
- **Repository / DDD / CQRS.** Domain types in `internal/<domain>/`; repository interfaces defined where consumed; storage details hidden. Direction, not enforced.
- **Migrations deviation.** `goose` instead of `golang-migrate`; rationale: already wired and equivalent for v1 needs.

## 9. Replication plan (after the slice lands)

For each of `solana-collector`, `probe-collector`, `publisher`:

1. Create `config/<binary>/base.toml` and `config/<binary>/local.toml`.
2. Rewrite `cmd/<binary>/main.go` mirroring the slice: parse config, init log, open db pool, log ready, exit.
3. Add the `APP_ENV` and `DATABASE__URL` env entries in `docker-compose.yml`.

No tests added for the replicas — they're stubs. The slice's tests cover the shared `internal/{config,log,db}` packages.

## 10. Out of scope

- No HTTP server, chi router, OTEL wiring, or REST handlers in code.
- No `internal/money/`, `internal/uuid/`, sqlc, repository abstractions, or domain packages.
- No real collector loops (`base-collector` still exits after logging ready).
- No CI workflow changes beyond what the layout demands (path updates only).
- No `git-cliff` setup.
- No K8s, no VPS work, no production deployment story.

## 11. Done criteria

- `just up` brings the stack up; `base-collector` logs a structured `"base-collector ready"` event via slog with config loaded from layered TOML; the binary opens a working pgx pool against compose Postgres and exits cleanly.
- `go test -race ./internal/...` passes (unit tests for `internal/config`).
- `go test -tags=integration -race ./...` passes (testcontainers brings up Postgres, applies goose migrations, opens pool, queries `SELECT 1`, checks `_fathom_meta`).
- `golangci-lint run`, `go tool govulncheck`, `lefthook run pre-commit` all clean.
- `docs/conventions.md` exists; `README.md` and `docs/architecture.md` have updated paths.
- The other three binaries (`solana-collector`, `probe-collector`, `publisher`) mirror the slice's main.go shape with their own config TOMLs.

## 12. Non-goals (explicit)

- Validating that the deferred libraries (apd, uuid, sqlc, chi, otel) are correctly callable. They land with their first real consumer.
- Refactoring the existing migrations, views, or docker-compose beyond what the path moves require.
- Adding observability infrastructure (OTEL collector, log aggregator, metrics endpoint) — v1 runs on one operator's laptop.
