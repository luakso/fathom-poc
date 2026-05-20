# Golang Reference Guide Alignment — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the fathom foundation to the golang-ref-guide posture — vertical slice through `base-collector` first (config → log → db), then replicate to the other three binaries, then write doc-only patterns. Stubs stay stubs; this is foundation, not feature.

**Architecture:** Three small `internal/` packages (`config`, `log`, `db`) plus per-binary `config/<binary>/*.toml`. Migrations/views/init-script consolidated under `/database/`. Each binary's `main.go` becomes the same shape: parse layered TOML, init slog, open pgx pool, log ready, exit.

**Tech Stack:** Go 1.26 (tool directive), `knadh/koanf/v2`, `log/slog` (stdlib), `jackc/pgx/v5`, `testcontainers-go/modules/postgres`, `pressly/goose/v3` (programmatic + CLI), Docker Compose, justfile, lefthook.

**Spec:** [`../specs/2026-05-20-golang-ref-alignment-design.md`](../specs/2026-05-20-golang-ref-alignment-design.md)

---

## Task 1: Move SQL and init script under `/database/`

Mechanical layout change. One commit, no behavior change. After this task, `just nuke && just up` must still apply migrations and views correctly.

**Files:**
- Move: `migrations/` → `database/migrations/` (including `00001_init.sql` and `.gitkeep`)
- Move: `views/` → `database/views/` (including `_smoke_v1.sql` and `.gitkeep`)
- Move: `scripts/init-db.sh` → `database/init/init-db.sh`
- Modify: `Dockerfile` (line 30: COPY path for init-db.sh)
- Modify: `docker-compose.yml` (lines 28-29: init-db volume mounts)
- Modify: `justfile` (line 76: goose `-dir` path; lines 81-85: views glob path)
- Modify: `README.md` (Layout section, lines ~52-62)
- Modify: `docs/architecture.md` (any path references — check `_smoke_v1` mention)

- [ ] **Step 1: Move the directories**

```bash
mkdir -p database
git mv migrations database/migrations
git mv views database/views
mkdir -p database/init
git mv scripts/init-db.sh database/init/init-db.sh
rmdir scripts 2>/dev/null || true
```

- [ ] **Step 2: Update `Dockerfile` line 30**

Change:
```dockerfile
COPY scripts/init-db.sh /init-db.sh
```
to:
```dockerfile
COPY database/init/init-db.sh /init-db.sh
```

- [ ] **Step 3: Update `docker-compose.yml` init-db volumes**

Change lines 27-29:
```yaml
    volumes:
      - ./migrations:/migrations:ro
      - ./views:/views:ro
```
to:
```yaml
    volumes:
      - ./database/migrations:/migrations:ro
      - ./database/views:/views:ro
```

(The container-side paths `/migrations` and `/views` stay the same — `init-db.sh` reads from those mount points.)

- [ ] **Step 4: Update `justfile` migrate recipe**

Change line 76 from:
```
        go tool goose -dir migrations postgres "$DB_URL_HOST" {{args}}
```
to:
```
        go tool goose -dir database/migrations postgres "$DB_URL_HOST" {{args}}
```

- [ ] **Step 5: Update `justfile` apply-views recipe**

Change lines 81-85 from:
```
apply-views:
    @set -a; . ./.env; set +a; \
        for f in views/*.sql; do \
            [ -e "$f" ] || break; \
            echo "applying $f"; \
            docker compose exec -T postgres psql "$DB_URL" -v ON_ERROR_STOP=1 -f - < "$f"; \
        done
```
to:
```
apply-views:
    @set -a; . ./.env; set +a; \
        for f in database/views/*.sql; do \
            [ -e "$f" ] || break; \
            echo "applying $f"; \
            docker compose exec -T postgres psql "$DB_URL" -v ON_ERROR_STOP=1 -f - < "$f"; \
        done
```

- [ ] **Step 6: Update `README.md` Layout section**

Replace the Layout code block (lines ~52-60):
```
cmd/<binary>/main.go   # one entrypoint per long-running unit
internal/              # shared packages (emerge with the first real collector)
migrations/            # goose .sql migrations — NNNNN_name.sql
views/                 # versioned SQL views — methodology lives here
scripts/init-db.sh     # one-shot: goose up + apply views (used inside init-db container)
docs/                  # spec, architecture, plans
```
with:
```
cmd/<binary>/main.go        # one entrypoint per long-running unit
config/<binary>/            # per-binary TOML (base.toml, local.toml, etc.)
internal/                   # shared packages (config, log, db)
database/migrations/        # goose .sql migrations — NNNNN_name.sql
database/views/             # versioned SQL views — methodology lives here
database/init/init-db.sh    # one-shot: goose up + apply views (inside init-db container)
database/testdata/          # seed scripts (empty in v1)
docs/                       # spec, architecture, plans, conventions
```

Also update the sentence below the code block — change `migrations/` and `views/` references to `database/migrations/` and `database/views/`.

- [ ] **Step 7: Update `docs/architecture.md` path references**

Search the doc for `migrations/`, `views/`, `scripts/init-db.sh`. The architecture content itself does not change — only update path references if any exist. The §5 invariant about `views/` directory should now read `database/views/` if it appears.

Run:
```bash
grep -n -E "(^|[^a-z/])(migrations|views|scripts/init-db)" docs/architecture.md
```
Update each match to the new path. If there are no matches, this step is a no-op.

- [ ] **Step 8: Verify `just up` end-to-end**

```bash
just nuke
just up
docker compose logs init-db | tail -20
```
Expected: init-db logs `init-db: running goose migrations`, applies `00001_init.sql`, then `init-db: applying views` and applies `_smoke_v1.sql`, then `init-db: done` and exits 0. The three collectors log their startup message (currently just `log.Println`) and exit. Postgres stays up.

Then verify the table and view exist:
```bash
just psql -c "\dt"
just psql -c "SELECT * FROM _smoke_v1;"
```
Expected: `_fathom_meta` table listed; `_smoke_v1` returns `ok | <timestamp>`.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "chore: move sql and init script under /database/"
```

---

## Task 2: Add new dependencies to go.mod

We need three new direct deps: `knadh/koanf/v2`, `jackc/pgx/v5`, `testcontainers-go`. We do NOT add `apd`, `uuid`, or `sqlc` — they have no consumer yet (per spec §10).

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Add the deps**

```bash
go get github.com/knadh/koanf/v2
go get github.com/knadh/koanf/parsers/toml/v2
go get github.com/knadh/koanf/providers/env/v2
go get github.com/knadh/koanf/providers/file
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
go mod tidy
```

- [ ] **Step 2: Verify they show up as direct requires**

```bash
go list -m -mod=mod github.com/knadh/koanf/v2 github.com/jackc/pgx/v5 github.com/testcontainers/testcontainers-go
```
Expected: each prints a version (no errors).

Also verify the `tool` directive in `go.mod` is unchanged — we did not remove `goose`, since the CLI is still used by `just migrate` and the integration test will invoke `goose` programmatically using the same module.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add koanf, pgx, testcontainers deps"
```

---

## Task 3: Implement `internal/config` (TDD)

Generic, layered config loader lifted from `golang-ref-guide/configuration.md`. We drop the `Observability` and `HTTP` sub-structs from `BasicConfig` because v1 has no consumer for them (spec §5.2). Unit-tested using temp dirs; no DB needed.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/errors.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lukostrobl/fathom/internal/config"
)

type testCfg struct {
	config.BasicConfig
	Database struct {
		URL string `koanf:"url"`
	} `koanf:"database"`
}

func (c testCfg) GetBasicConfig() config.BasicConfig { return c.BasicConfig }

// writeCfg writes a config tree rooted at dir/config/<binary>/ and returns dir.
// chdir into dir before calling ParseConfig so the relative paths in the loader resolve.
func writeCfg(t *testing.T, binary, base, env, secrets string) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "config", binary)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if base != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "base.toml"), []byte(base), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if env != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "local.toml"), []byte(env), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if secrets != "" {
		if err := os.WriteFile(filepath.Join(cfgDir, "local.secrets.toml"), []byte(secrets), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestParseConfig_LoadsBaseAndEnv(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" + `[database]` + "\n" + `url = "postgres://x"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Name != "x" || cfg.Version != "v0" || cfg.Env != "local" {
		t.Errorf("BasicConfig fields wrong: %+v", cfg.BasicConfig)
	}
	if cfg.Database.URL != "postgres://x" {
		t.Errorf("Database.URL = %q, want postgres://x", cfg.Database.URL)
	}
}

func TestParseConfig_EnvVarOverridesTOML(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n" + `[log]` + "\n" + `level = "info"` + "\n"
	env := `env = "local"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	t.Setenv("LOG__LEVEL", "warn")

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want warn (env override)", cfg.Log.Level)
	}
}

func TestParseConfig_SecretsFileOverlays(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" +
		`secrets_path = "config/x/local.secrets.toml"` + "\n" +
		`[database]` + "\n" +
		`url = "postgres://placeholder"` + "\n"
	secrets := `[database]` + "\n" + `url = "postgres://real"` + "\n"
	dir := writeCfg(t, "x", base, env, secrets)
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Database.URL != "postgres://real" {
		t.Errorf("Database.URL = %q, want postgres://real", cfg.Database.URL)
	}
}

func TestParseConfig_SecretsFileMissingIsSilent(t *testing.T) {
	base := `name = "x"` + "\n" + `version = "v0"` + "\n"
	env := `env = "local"` + "\n" + `secrets_path = "config/x/local.secrets.toml"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	cfg, err := config.ParseConfig[testCfg]("x", "local")
	if err != nil {
		t.Fatalf("ParseConfig: %v (secrets file absent must be silent)", err)
	}
	if cfg.Name != "x" {
		t.Errorf("BasicConfig.Name = %q", cfg.Name)
	}
}

func TestParseConfig_MissingRequiredFields(t *testing.T) {
	// base.toml has no name/version; <env>.toml only sets env.
	base := ""
	env := `env = "local"` + "\n"
	dir := writeCfg(t, "x", base, env, "")
	chdir(t, dir)

	_, err := config.ParseConfig[testCfg]("x", "local")
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	var missing config.MissingRequiredFieldsError
	if !errors.As(err, &missing) {
		t.Errorf("error type = %T, want MissingRequiredFieldsError", err)
	}
}

func TestParseConfig_MissingBaseFile(t *testing.T) {
	// no base.toml, no env.toml
	dir := t.TempDir()
	chdir(t, dir)
	_, err := config.ParseConfig[testCfg]("x", "local")
	if err == nil {
		t.Fatal("expected error when base.toml is missing")
	}
}
```

- [ ] **Step 2: Run the tests to confirm they fail to compile**

Run:
```bash
go test ./internal/config/... 2>&1 | head -20
```
Expected: compile error — `package github.com/lukostrobl/fathom/internal/config` does not exist (or no Go files).

- [ ] **Step 3: Implement `internal/config/errors.go`**

Create `internal/config/errors.go`:

```go
package config

import "fmt"

// MissingRequiredFieldsError is returned by ParseConfig when one of the
// mandatory BasicConfig fields (Env, Name, Version) is empty after all four
// layers have been merged.
type MissingRequiredFieldsError struct {
	BinaryName string
	Env        string
	Name       string
	Version    string
}

func (m MissingRequiredFieldsError) Error() string {
	return fmt.Sprintf(
		"missing required fields in config(%s, %s): name=%q version=%q env=%q",
		m.BinaryName, m.Env, m.Name, m.Version,
	)
}
```

- [ ] **Step 4: Implement `internal/config/config.go`**

Create `internal/config/config.go`:

```go
// Package config loads layered TOML config for fathom binaries.
//
// Layers, each overriding the previous:
//   1. config/<binary>/base.toml
//   2. config/<binary>/<env>.toml
//   3. environment variables (FOO__BAR -> foo.bar)
//   4. file at BasicConfig.SecretsPath, if it exists
//
// Generic over a per-binary Config type that embeds BasicConfig.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// BasicConfigurator is the contract every per-binary Config struct must satisfy
// so ParseConfig can validate the shared mandatory fields.
type BasicConfigurator interface {
	GetBasicConfig() BasicConfig
}

// BasicConfig holds fields every binary needs. Per-binary Config structs embed
// this and add their own sections.
type BasicConfig struct {
	Env     string `koanf:"env"`
	Name    string `koanf:"name"`
	Version string `koanf:"version"`

	SecretsPath string `koanf:"secrets_path"`

	Log struct {
		IsPretty bool   `koanf:"is_pretty"`
		Level    string `koanf:"level"`
	} `koanf:"log"`

	WithDebugProfiler bool `koanf:"with_debug_profiler"`
}

// ParseConfig loads the four layers and unmarshals into Config. The binaryName
// determines the config directory (config/<binaryName>/), and environment
// selects which env-specific overlay (config/<binaryName>/<environment>.toml).
func ParseConfig[Config BasicConfigurator](binaryName, environment string) (Config, error) {
	var empty Config
	k := koanf.New(".")

	basePath := fmt.Sprintf("config/%s/base.toml", binaryName)
	if err := k.Load(file.Provider(basePath), toml.Parser()); err != nil {
		return empty, fmt.Errorf("load base config (%s, %s): %w", binaryName, environment, err)
	}

	envPath := fmt.Sprintf("config/%s/%s.toml", binaryName, environment)
	if err := k.Load(file.Provider(envPath), toml.Parser()); err != nil {
		return empty, fmt.Errorf("load env config (%s, %s): %w", binaryName, environment, err)
	}

	if err := k.Load(env.Provider("", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(s), "__", ".")
	}), nil); err != nil {
		return empty, fmt.Errorf("load env vars (%s, %s): %w", binaryName, environment, err)
	}

	if val, ok := k.Get("secrets_path").(string); ok && val != "" {
		if _, err := os.Stat(val); err == nil {
			if err := k.Load(file.Provider(val), toml.Parser()); err != nil {
				return empty, fmt.Errorf("load secrets %s: %w", val, err)
			}
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return empty, fmt.Errorf("unmarshal config (%s, %s): %w", binaryName, environment, err)
	}

	b := cfg.GetBasicConfig()
	if b.Name == "" || b.Version == "" || b.Env == "" {
		return empty, MissingRequiredFieldsError{
			BinaryName: binaryName, Env: b.Env, Name: b.Name, Version: b.Version,
		}
	}
	return cfg, nil
}
```

Note: koanf's env provider v2 takes a key transform; the env.Provider signature in v2 is `env.Provider(prefix, delimiter, callback)`. If `go build` fails because of a signature mismatch (the koanf API has shifted across versions), inspect `go doc github.com/knadh/koanf/providers/env/v2 Provider` and adapt the call accordingly — the semantic is unchanged: lowercase + replace `__` with `.`.

- [ ] **Step 5: Run the tests — they should pass**

```bash
go test -race ./internal/config/...
```
Expected: all six tests pass. If `env.Provider` signature differs, fix it and re-run.

- [ ] **Step 6: Run gofumpt, goimports, golangci-lint**

```bash
just fmt
just lint
```
Expected: no errors. Fix any lint issues before committing.

- [ ] **Step 7: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): layered TOML loader with BasicConfig"
```

---

## Task 4: Implement `internal/log`

Trivial wrapper that builds an `*slog.Logger` from `BasicConfig.Log`. No unit test — the body is too small to test independently and the integration test exercises it.

**Files:**
- Create: `internal/log/log.go`

- [ ] **Step 1: Implement `internal/log/log.go`**

```go
// Package log builds an *slog.Logger from BasicConfig.Log.
package log

import (
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
)

// New returns a slog.Logger configured per BasicConfig.Log and also installs it
// as the slog package default so plain `slog.Info(...)` calls inherit the
// chosen handler.
func New(b config.BasicConfig) *slog.Logger {
	level := parseLevel(b.Log.Level)
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if b.Log.IsPretty {
		handler = slog.NewTextHandler(os.Stderr, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	}

	logger := slog.New(handler).With("binary", b.Name, "version", b.Version, "env", b.Env)
	slog.SetDefault(logger)
	return logger
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/log/...
```
Expected: success, no output.

- [ ] **Step 3: Format and lint**

```bash
just fmt
just lint
```
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/log/
git commit -m "feat(log): slog setup driven by BasicConfig"
```

---

## Task 5: Implement `internal/db`

Single function: open a `*pgxpool.Pool` from a connection URL. No unit test — needs a real DB; the integration test in Task 9 exercises it.

**Files:**
- Create: `internal/db/db.go`

- [ ] **Step 1: Implement `internal/db/db.go`**

```go
// Package db opens a pgx connection pool.
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Open returns a pgxpool.Pool connected to the given URL. The caller is
// responsible for calling Close on the returned pool.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open db pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/db/...
```
Expected: success.

- [ ] **Step 3: Format and lint**

```bash
just fmt
just lint
```

- [ ] **Step 4: Commit**

```bash
git add internal/db/
git commit -m "feat(db): pgxpool open helper"
```

---

## Task 6: Create `config/base-collector/` TOML files

Per spec §5.1. Two checked-in files; the secrets file is added to `.gitignore` (it's never written here).

**Files:**
- Create: `config/base-collector/base.toml`
- Create: `config/base-collector/local.toml`
- Modify: `.gitignore` (add `local.secrets.toml` pattern)

- [ ] **Step 1: Write `config/base-collector/base.toml`**

```toml
name = "base-collector"
version = "v0"
with_debug_profiler = false

[log]
is_pretty = false
level = "info"
```

- [ ] **Step 2: Write `config/base-collector/local.toml`**

```toml
env = "local"
secrets_path = "config/base-collector/local.secrets.toml"

[log]
is_pretty = true
level = "debug"
```

(`[database]` is intentionally absent here — `database.url` arrives via the `DATABASE__URL` env var set by docker-compose.)

- [ ] **Step 3: Add the gitignore pattern**

Append to `.gitignore` under the `# Env / secrets` section:

```
config/*/local.secrets.toml
config/*/*.secrets.toml
```

- [ ] **Step 4: Commit**

```bash
git add config/base-collector/ .gitignore
git commit -m "chore(config): add base-collector TOML overlays"
```

---

## Task 7: Rewrite `cmd/base-collector/main.go`

Wire config → log → db. Still a stub — exits after logging ready.

**Files:**
- Modify: `cmd/base-collector/main.go` (replace contents entirely)

- [ ] **Step 1: Replace the file**

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
)

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

	logger.Info("base-collector ready")
}
```

- [ ] **Step 2: Verify it builds**

```bash
go build ./cmd/base-collector/...
```
Expected: success.

- [ ] **Step 3: Format and lint**

```bash
just fmt
just lint
```

- [ ] **Step 4: Commit**

```bash
git add cmd/base-collector/main.go
git commit -m "feat(base-collector): wire config, log, pgx pool"
```

---

## Task 8: Bake configs into runtime image; wire env in compose

The runtime stage currently only contains the binary. After this task, `config/` is also present in the image, and compose sets `APP_ENV` and `DATABASE__URL` per collector.

**Files:**
- Modify: `Dockerfile` (runtime stage, lines ~20-24)
- Modify: `docker-compose.yml` (env block under each collector service)
- Modify: `.env.example` (add `APP_ENV`)

- [ ] **Step 1: Update `Dockerfile` runtime stage**

Replace lines 20-24:
```dockerfile
# ---- Runtime for Go binaries ----
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=builder /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
```
with:
```dockerfile
# ---- Runtime for Go binaries ----
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /
COPY --from=builder /out/app /app
COPY --from=builder /src/config /config
USER nonroot:nonroot
ENTRYPOINT ["/app"]
```

The `config/` directory is baked in so the binary can resolve `config/<binary>/base.toml` relative to its working directory (`/`).

- [ ] **Step 2: Update `docker-compose.yml` for `base-collector`**

Find the `base-collector` service block (around lines 35-47). Change its `environment:` from:
```yaml
    environment:
      DB_URL: ${DB_URL}
```
to:
```yaml
    environment:
      APP_ENV: local
      DATABASE__URL: ${DB_URL}
```

(The other three collectors still use `DB_URL` — they'll be updated when we replicate the slice in Tasks 11-13.)

- [ ] **Step 3: Add `APP_ENV` to `.env.example`**

Append at the bottom:

```

# Binary environment (selects config/<binary>/<env>.toml overlay)
APP_ENV=local
```

- [ ] **Step 4: Verify `just up` brings `base-collector` up with the new wiring**

```bash
just nuke
just up
docker compose logs base-collector
```
Expected: a single JSON-or-text slog line like:
```
time=... level=INFO msg="base-collector ready" binary=base-collector version=v0 env=local
```
(Pretty text because `local.toml` sets `is_pretty=true`.) Container exits 0.

If config loading fails, `docker compose logs base-collector` will show the slog error. Common cause: `config/` not present in the runtime image — verify with `docker compose run --rm base-collector ls config/base-collector` (should list `base.toml` and `local.toml`).

- [ ] **Step 5: Commit**

```bash
git add Dockerfile docker-compose.yml .env.example
git commit -m "feat(compose): bake configs into image, wire APP_ENV/DATABASE__URL for base-collector"
```

---

## Task 9: Integration test using testcontainers

Write a tag-gated integration test that brings up a real Postgres via testcontainers, applies goose migrations programmatically, and exercises `db.Open`.

**Files:**
- Create: `cmd/base-collector/integration_test.go`
- Modify: `justfile` (add `test-integration` recipe)

- [ ] **Step 1: Write the failing test**

Create `cmd/base-collector/integration_test.go`:

```go
//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql, used by goose
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	tcwait "github.com/testcontainers/testcontainers-go/wait"

	"github.com/lukostrobl/fathom/internal/db"
)

func TestBaseCollector_OpensPoolAgainstRealPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("fathom_test"),
		postgres.WithUsername("fathom"),
		postgres.WithPassword("fathom"),
		postgres.BasicWaitStrategies(),
		postgres.WithWaitStrategy(tcwait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = pg.Terminate(ctx)
	})

	connStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("conn string: %v", err)
	}

	// Apply goose migrations using database/sql + pgx stdlib driver.
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer sqlDB.Close()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose SetDialect: %v", err)
	}
	if err := goose.Up(sqlDB, "../../database/migrations"); err != nil {
		t.Fatalf("goose Up: %v", err)
	}

	// Now exercise the unit under test.
	pool, err := db.Open(ctx, connStr)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer pool.Close()

	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", one)
	}

	var initialized string
	row := pool.QueryRow(ctx, "SELECT value FROM _fathom_meta WHERE key = 'schema_initialized'")
	if err := row.Scan(&initialized); err != nil {
		t.Fatalf("read _fathom_meta: %v", err)
	}
	if initialized != "true" {
		t.Fatalf("_fathom_meta schema_initialized = %q, want true", initialized)
	}

	_ = stdlib.GetDefaultDriver() // silences unused-import linter without runtime cost
}
```

- [ ] **Step 2: Add deps the test needs**

```bash
go get github.com/pressly/goose/v3
go get github.com/jackc/pgx/v5/stdlib
go mod tidy
```

(Goose is already in the `tool` directive for CLI usage; this also pulls it as a regular module for programmatic use.)

- [ ] **Step 3: Run the test to confirm it passes**

Make sure Docker Desktop is running (testcontainers needs a Docker daemon):

```bash
go test -tags=integration -race -v ./cmd/base-collector/...
```
Expected: the test starts a postgres:16-alpine container, runs the migration, opens the pool, asserts SELECT 1 = 1 and that `_fathom_meta.schema_initialized = "true"`, then exits with PASS. Takes ~30-60s on first run.

If `postgres.Run` signature differs from this code, check `go doc github.com/testcontainers/testcontainers-go/modules/postgres Run` — the helper has evolved across versions. Adapt the call accordingly; the test logic does not change.

- [ ] **Step 4: Add `test-integration` recipe to `justfile`**

Append to `justfile`:

```
# Run integration tests (requires Docker daemon for testcontainers)
test-integration:
    go test -tags=integration -race -v ./...
```

- [ ] **Step 5: Format and lint**

```bash
just fmt
just lint
```

Note: the unit test target `just test` does NOT pick up integration tests (no `-tags=integration`), so the default test path stays fast.

- [ ] **Step 6: Commit**

```bash
git add cmd/base-collector/integration_test.go justfile go.mod go.sum
git commit -m "test(base-collector): integration test against real postgres via testcontainers"
```

---

## Task 10: Replicate slice — `solana-collector`

Same shape as base-collector. No integration test added.

**Files:**
- Create: `config/solana-collector/base.toml`
- Create: `config/solana-collector/local.toml`
- Modify: `cmd/solana-collector/main.go`
- Modify: `docker-compose.yml` (solana-collector env block)

- [ ] **Step 1: Write `config/solana-collector/base.toml`**

```toml
name = "solana-collector"
version = "v0"
with_debug_profiler = false

[log]
is_pretty = false
level = "info"
```

- [ ] **Step 2: Write `config/solana-collector/local.toml`**

```toml
env = "local"
secrets_path = "config/solana-collector/local.secrets.toml"

[log]
is_pretty = true
level = "debug"
```

- [ ] **Step 3: Rewrite `cmd/solana-collector/main.go`**

Replace contents with the same template as base-collector, swapping the binary name:

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
)

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

	cfg, err := config.ParseConfig[Config]("solana-collector", env)
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

	logger.Info("solana-collector ready")
}
```

- [ ] **Step 4: Update `docker-compose.yml` for `solana-collector`**

In the `solana-collector` service block, change the `environment:` from:
```yaml
    environment:
      DB_URL: ${DB_URL}
```
to:
```yaml
    environment:
      APP_ENV: local
      DATABASE__URL: ${DB_URL}
```

- [ ] **Step 5: Verify `just up` boots solana-collector with the new wiring**

```bash
just nuke
just up
docker compose logs solana-collector
```
Expected: slog "solana-collector ready" line, container exits 0.

- [ ] **Step 6: Format, lint, commit**

```bash
just fmt
just lint
git add config/solana-collector/ cmd/solana-collector/main.go docker-compose.yml
git commit -m "feat(solana-collector): wire config, log, pgx pool"
```

---

## Task 11: Replicate slice — `probe-collector`

Same as Task 10, swap names.

**Files:**
- Create: `config/probe-collector/base.toml`
- Create: `config/probe-collector/local.toml`
- Modify: `cmd/probe-collector/main.go`
- Modify: `docker-compose.yml` (probe-collector env block)

- [ ] **Step 1: Write `config/probe-collector/base.toml`**

```toml
name = "probe-collector"
version = "v0"
with_debug_profiler = false

[log]
is_pretty = false
level = "info"
```

- [ ] **Step 2: Write `config/probe-collector/local.toml`**

```toml
env = "local"
secrets_path = "config/probe-collector/local.secrets.toml"

[log]
is_pretty = true
level = "debug"
```

- [ ] **Step 3: Rewrite `cmd/probe-collector/main.go`**

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
)

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

	cfg, err := config.ParseConfig[Config]("probe-collector", env)
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

	logger.Info("probe-collector ready")
}
```

- [ ] **Step 4: Update `docker-compose.yml` for `probe-collector`**

Change its `environment:` to:
```yaml
    environment:
      APP_ENV: local
      DATABASE__URL: ${DB_URL}
```

- [ ] **Step 5: Verify**

```bash
just nuke
just up
docker compose logs probe-collector
```
Expected: slog "probe-collector ready", exit 0.

- [ ] **Step 6: Format, lint, commit**

```bash
just fmt
just lint
git add config/probe-collector/ cmd/probe-collector/main.go docker-compose.yml
git commit -m "feat(probe-collector): wire config, log, pgx pool"
```

---

## Task 12: Replicate slice — `publisher`

Same template; publisher's compose entry is under the `manual` profile so the start verification is a `docker compose run --rm`.

**Files:**
- Create: `config/publisher/base.toml`
- Create: `config/publisher/local.toml`
- Modify: `cmd/publisher/main.go`
- Modify: `docker-compose.yml` (publisher env block)

- [ ] **Step 1: Write `config/publisher/base.toml`**

```toml
name = "publisher"
version = "v0"
with_debug_profiler = false

[log]
is_pretty = false
level = "info"
```

- [ ] **Step 2: Write `config/publisher/local.toml`**

```toml
env = "local"
secrets_path = "config/publisher/local.secrets.toml"

[log]
is_pretty = true
level = "debug"
```

- [ ] **Step 3: Rewrite `cmd/publisher/main.go`**

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/lukostrobl/fathom/internal/config"
	"github.com/lukostrobl/fathom/internal/db"
	applog "github.com/lukostrobl/fathom/internal/log"
)

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

	cfg, err := config.ParseConfig[Config]("publisher", env)
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

	logger.Info("publisher ready")
}
```

- [ ] **Step 4: Update `docker-compose.yml` for `publisher`**

Change its `environment:` to:
```yaml
    environment:
      APP_ENV: local
      DATABASE__URL: ${DB_URL}
```

- [ ] **Step 5: Verify with `docker compose run` (publisher is in the `manual` profile)**

```bash
just up
docker compose run --rm publisher
```
Expected: slog "publisher ready" line; exit 0.

- [ ] **Step 6: Format, lint, commit**

```bash
just fmt
just lint
git add config/publisher/ cmd/publisher/main.go docker-compose.yml
git commit -m "feat(publisher): wire config, log, pgx pool"
```

---

## Task 13: Write `docs/conventions.md`

Doc-only patterns from spec §8.

**Files:**
- Create: `docs/conventions.md`

- [ ] **Step 1: Write the doc**

Create `docs/conventions.md`:

```markdown
# Conventions

Patterns adopted from [`../../golang-ref-guide/`](../../golang-ref-guide/) that don't yet have a consumer in fathom code, but that future work will follow without re-deciding. When a section's code does land, this file is the authority for *how* it lands.

## Migrations — goose (deviation)

Fathom uses [`pressly/goose`](https://github.com/pressly/goose) rather than the reference's `golang-migrate`. Already wired into `go.mod` (tool directive), the init-db container, and the justfile. The two tools are functionally equivalent for v1's needs; the deviation is acknowledged here rather than rewriting working code.

## REST API

When the first HTTP surface lands (not in v1), follow [`../../golang-ref-guide/restapi.md`](../../golang-ref-guide/restapi.md):

- `net/http` server with explicit `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, and graceful shutdown.
- [`go-chi/chi`](https://go-chi.io) router. **Not** a context-based router — it does not play well with middleware stacks.
- URL versioning (`/v1/...`) plus a date header for non-breaking-but-tracked changes, Stripe/Shopify-style.
- net/http-compatible middleware only.
- Handlers built as closures that receive their deps via constructor. No package-level globals.

## Errors

Follow [`../../golang-ref-guide/errors.md`](../../golang-ref-guide/errors.md):

- Simple errors as typed string consts (the `marshalError string` pattern) so they're matchable via `errors.Is` without runtime init cost.
- Context-bearing errors as structs implementing `Error() string`.
- Wrap with `fmt.Errorf("...: %w", err)` whenever adding context.
- Prefer `errors.AsType[*FooError]` (Go 1.26 generic) over `errors.As` in new code.
- Errors that are part of a package's external API are typed and exported.

## OTEL tracing

When tracing infrastructure exists (deferred past v1):

- [`opentelemetry-go`](https://github.com/open-telemetry/opentelemetry-go) SDK.
- [`otelhttp`](https://github.com/open-telemetry/opentelemetry-go-contrib/tree/main/instrumentation/net/http/otelhttp) for outbound HTTP clients.
- [`otelchi`](https://github.com/riandyrn/otelchi) middleware on chi routers.
- Collector address read from a future `BasicConfig.Observability` sub-struct (intentionally absent from `BasicConfig` today — add it when the first span is emitted).

## Money

When `payments.amount` or any monetary value enters the schema:

- [`cockroachdb/apd/v3`](https://github.com/cockroachdb/apd) for arbitrary-precision decimals.
- Always carry an `apd.Context` explicitly: `apd.BaseContext.WithPrecision(20)` for money.
- pgx `pgtype.Numeric` round-trips to `*apd.Decimal` directly — no glue code.
- For JSON APIs (when they exist), wrap to marshal as string, not as a JSON number.

## UUID

- [`google/uuid`](https://github.com/google/uuid).
- **Never** [`satori/go.uuid`](https://github.com/satori/go.uuid) (historical security issues).

## Domain / repository / CQRS

When domain logic emerges (collector loops, classification, methodology):

- Domain types under `internal/<domain>/` (e.g. `internal/payments/`, `internal/services/`).
- Repository interfaces defined where they're consumed, not where they're implemented (the threedots.tech pattern).
- Storage details (pgx queries, sqlc-generated code) hidden behind the interface so collector logic can be unit-tested with in-memory fakes.
- CQRS is the direction (commands separate from queries) but not enforced before there's a domain to model.

## Tests

- Unit tests use stdlib `testing` only, table-driven where it pays off.
- Integration tests guarded by `//go:build integration` and run via `just test-integration`. They bring up real Postgres via [`testcontainers-go`](https://golang.testcontainers.org/) — never via the dev `docker compose`.
- Always `-race`.
```

- [ ] **Step 2: Commit**

```bash
git add docs/conventions.md
git commit -m "docs: conventions for deferred patterns (REST, OTEL, money, uuid, repo)"
```

---

## Task 14: Update top-level docs and `.env.example`

Trailing cleanup: ensure README, architecture doc, and `.env.example` reflect reality. `.env.example` was already touched in Task 8, but a final consistency pass is cheap.

**Files:**
- Modify: `README.md` (Daily commands section: add `just test-integration` and `just dev`)
- Verify: `docs/architecture.md` (paths from Task 1 — should already be correct)
- Verify: `.env.example` (APP_ENV from Task 8 — should already be correct)

- [ ] **Step 1: Add the new justfile recipes to `README.md` Daily commands**

In `README.md`'s Daily commands code block, add these lines next to `just test`:

```
just test                # go test -race (unit)
just test-integration    # go test -tags=integration (real postgres via testcontainers)
just dev <binary>        # run a binary from the host with APP_ENV=local
```

Replace the existing `just test          # go test -race` line.

(The `just dev` recipe was promised in the spec but only added if Task 9's commit didn't include it. Verify with `grep '^dev' justfile`. If absent, add this recipe to justfile:

```
# Run a binary from the host with APP_ENV=local (outside docker)
dev binary:
    APP_ENV=local go run ./cmd/{{binary}}
```

and stage it together with the README change.)

- [ ] **Step 2: Sanity check architecture doc and .env.example**

```bash
grep -n -E "(^|[^a-z/])(migrations|views|scripts/init-db)" docs/architecture.md README.md
grep '^APP_ENV' .env.example
```
Expected: no matches in `docs/architecture.md` and `README.md` pointing to the OLD `migrations/`, `views/`, or `scripts/init-db.sh` paths. `.env.example` contains `APP_ENV=local`.

If any stale references remain, fix them in this commit.

- [ ] **Step 3: Final end-to-end verification**

```bash
just nuke
just up
docker compose logs init-db
docker compose logs base-collector solana-collector probe-collector
docker compose run --rm publisher
just psql -c "SELECT * FROM _smoke_v1;"
go test -race ./...
just test-integration   # if Docker is available
just lint
just vuln
```

Expected:
- init-db applies migrations + views, exits 0
- each collector logs `<binary> ready` via slog and exits 0
- publisher (manual profile) logs `publisher ready` and exits 0
- `_smoke_v1` returns `ok | <timestamp>`
- unit tests pass
- integration test passes
- lint, vuln clean

- [ ] **Step 4: Commit**

```bash
git add README.md justfile
git commit -m "docs: update README for new justfile recipes"
```

---

## Self-Review (done by the planner before handoff)

**Spec coverage:**

| Spec section | Implemented by task(s) |
|---|---|
| §2 Decisions | Locked in plan structure (vertical slice, goose kept, /database/ layout, bake configs) |
| §3 Adoption matrix | Tasks 2 (deps), 3-5 (internal packages), 13 (doc-only) |
| §4 Target layout | Task 1 (database/), Tasks 3-5 (internal/), Tasks 6,10,11,12 (config/) |
| §5.1 base-collector TOMLs | Task 6 |
| §5.2 internal/config | Task 3 |
| §5.3 internal/log | Task 4 |
| §5.4 internal/db | Task 5 |
| §5.5 base-collector main.go | Task 7 |
| §5.6 Tests (unit + integration) | Task 3 (unit), Task 9 (integration) |
| §6 File moves | Task 1 |
| §7 Compose / Dockerfile | Task 8 |
| §8 docs/conventions.md | Task 13 |
| §9 Replication | Tasks 10, 11, 12 |
| §10 Out of scope | Honored — no apd, uuid, sqlc, chi, OTEL, real loops |
| §11 Done criteria | Task 14 Step 3 verifies all six bullets |
| §12 Non-goals | Plan does not include any of them |

All covered.

**Placeholder scan:** No TBDs, no "implement later", no "similar to Task N", no "add appropriate error handling". Every code step shows code; every command step shows the command and expected output.

**Type consistency:** `Config`, `BasicConfig`, `BasicConfigurator`, `ParseConfig`, `Open`, `New`, `MissingRequiredFieldsError` — names match across Tasks 3-12. The `Config` struct in each of `cmd/<binary>/main.go` (Tasks 7, 10, 11, 12) has identical shape. Import path `applog "github.com/lukostrobl/fathom/internal/log"` is consistent across all four `main.go` files.

Plan complete.
