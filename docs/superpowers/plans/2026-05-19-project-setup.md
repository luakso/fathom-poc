# Project Setup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Fathom Go monorepo with linting, formatting, pre-commit hooks, task runner, migrations, Docker dev environment, and CI — no business logic, just the rails any future code will run on.

**Architecture:** Single Go module at repo root; four `cmd/<name>/main.go` binaries built from one multi-stage Dockerfile; Postgres + binaries + a one-shot `init-db` container orchestrated by `docker compose`; `just` as the task runner; `lefthook` as the pre-commit hook manager; tool binaries (gofumpt, goimports, golangci-lint, goose, govulncheck) pinned via Go 1.24+ `tool` directives in `go.mod`.

**Tech Stack:** Go (latest stable), Postgres 16, Docker + Compose, `just`, `lefthook`, `golangci-lint`, `gofumpt`, `goimports`, `goose`, `govulncheck`, GitHub Actions.

**Spec:** [`docs/superpowers/specs/2026-05-19-project-setup-design.md`](../specs/2026-05-19-project-setup-design.md)

---

## Prerequisites Note

This plan executes on macOS (Darwin) with Homebrew available. `git`, `docker`, `docker compose`, and `just` are already installed. `go` and `lefthook` are not — Task 0 installs them.

---

## Task 0: Install missing prerequisites

**Files:** none

- [ ] **Step 1: Install Go via Homebrew**

Run:
```bash
brew install go
```

Verify:
```bash
go version
```
Expected: prints `go version go1.XX.Y darwin/...` where XX ≥ 24.

- [ ] **Step 2: Install lefthook via Homebrew**

Run:
```bash
brew install lefthook
```

Verify:
```bash
lefthook version
```
Expected: prints a version string.

- [ ] **Step 3: No commit (nothing to commit yet — repo not initialized)**

---

## Task 1: Initialize git repo + foundational ignores

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/.gitignore`
- Create: `/Users/lukasstrobl/Developer/fathom/.editorconfig`
- Create: `/Users/lukasstrobl/Developer/fathom/.dockerignore`

- [ ] **Step 1: Init the repo**

Run from `/Users/lukasstrobl/Developer/fathom`:
```bash
git init -b main
```
Expected: `Initialized empty Git repository in .../.git/`.

- [ ] **Step 2: Create `.gitignore`**

```gitignore
# Build artifacts
/bin/
/dist/
/out/
*.exe
*.test
*.out
coverage.txt
coverage.html

# Go workspace
go.work
go.work.sum

# Env / secrets
.env
.env.*
!.env.example

# Docker overrides
compose.override.yml
docker-compose.override.yml

# OS / IDE
.DS_Store
.idea/
.vscode/
*.swp
```

- [ ] **Step 3: Create `.editorconfig`**

```ini
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 2

[*.go]
indent_style = tab
indent_size = 4

[Makefile]
indent_style = tab

[*.md]
trim_trailing_whitespace = false
```

- [ ] **Step 4: Create `.dockerignore`**

```dockerignore
.git/
.github/
docs/
.idea/
.vscode/
.DS_Store
*.md
.env
.env.*
compose.override.yml
docker-compose.override.yml
bin/
dist/
out/
```

- [ ] **Step 5: First commit**

```bash
git add .gitignore .editorconfig .dockerignore CLAUDE.md docs/
git commit -m "chore: initialize repo with ignore files"
```

Expected: commit succeeds. `git log --oneline` shows one commit.

---

## Task 2: Initialize Go module + create cmd/ binary stubs

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/go.mod` (via `go mod init`)
- Create: `/Users/lukasstrobl/Developer/fathom/cmd/base-collector/main.go`
- Create: `/Users/lukasstrobl/Developer/fathom/cmd/solana-collector/main.go`
- Create: `/Users/lukasstrobl/Developer/fathom/cmd/probe-collector/main.go`
- Create: `/Users/lukasstrobl/Developer/fathom/cmd/publisher/main.go`

- [ ] **Step 1: Initialize the module**

Run from `/Users/lukasstrobl/Developer/fathom`:
```bash
go mod init github.com/lukostrobl/fathom
```

Verify `go.mod` exists and starts with `module github.com/lukostrobl/fathom`.

- [ ] **Step 2: Create `cmd/base-collector/main.go`**

```go
package main

import "log"

func main() {
	log.Println("base-collector: starting")
}
```

- [ ] **Step 3: Create `cmd/solana-collector/main.go`**

```go
package main

import "log"

func main() {
	log.Println("solana-collector: starting")
}
```

- [ ] **Step 4: Create `cmd/probe-collector/main.go`**

```go
package main

import "log"

func main() {
	log.Println("probe-collector: starting")
}
```

- [ ] **Step 5: Create `cmd/publisher/main.go`**

```go
package main

import "log"

func main() {
	log.Println("publisher: starting")
}
```

- [ ] **Step 6: Verify all four build**

Run:
```bash
go build ./cmd/...
```
Expected: exits 0, no output.

- [ ] **Step 7: Commit**

```bash
git add go.mod cmd/
git commit -m "feat: add go module and cmd stubs for four binaries"
```

---

## Task 3: Add formatter + linter tools (gofumpt, goimports, golangci-lint, govulncheck)

**Files:**
- Modify: `/Users/lukasstrobl/Developer/fathom/go.mod` (add tool directives)
- Create: `/Users/lukasstrobl/Developer/fathom/.golangci.yml`

- [ ] **Step 1: Add gofumpt as a tool**

Run:
```bash
go get -tool mvdan.cc/gofumpt@latest
```
Expected: `go.mod` now contains a `tool mvdan.cc/gofumpt` line. `go.sum` updated.

- [ ] **Step 2: Add goimports as a tool**

Run:
```bash
go get -tool golang.org/x/tools/cmd/goimports@latest
```

- [ ] **Step 3: Add golangci-lint as a tool**

Run:
```bash
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

- [ ] **Step 4: Add govulncheck as a tool**

Run:
```bash
go get -tool golang.org/x/vuln/cmd/govulncheck@latest
```

- [ ] **Step 5: Create `.golangci.yml`**

```yaml
version: "2"

run:
  timeout: 5m
  go: "1.24"

linters:
  default: none
  enable:
    - errcheck
    - govet
    - staticcheck
    - ineffassign
    - unused
    - gosec
    - revive
    - gocritic
  settings:
    revive:
      rules:
        - name: var-naming
        - name: exported
          arguments:
            - disableStutteringCheck
    gosec:
      excludes:
        - G104  # duplicated by errcheck
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - gosec
          - errcheck

formatters:
  enable:
    - gofumpt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - github.com/lukostrobl/fathom
```

- [ ] **Step 6: Verify gofumpt runs**

Run:
```bash
go tool gofumpt -l .
```
Expected: exits 0 with no file list (all four `main.go` files are already gofumpt-clean as written).

- [ ] **Step 7: Verify golangci-lint runs clean**

Run:
```bash
go tool golangci-lint run ./...
```
Expected: exits 0 with no issues.

- [ ] **Step 8: Verify govulncheck runs clean**

Run:
```bash
go tool govulncheck ./...
```
Expected: exits 0 with "No vulnerabilities found."

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum .golangci.yml
git commit -m "chore: pin gofumpt/goimports/golangci-lint/govulncheck as go tools"
```

---

## Task 4: Create the justfile

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/justfile`

- [ ] **Step 1: Write the `justfile` (Docker recipes come in later tasks; this is the Go-side recipes only)**

```just
# Default: show recipes
default:
    @just --list

# Format Go files
fmt:
    go tool gofumpt -w .
    go tool goimports -w .

# Lint Go files
lint:
    go tool golangci-lint run ./...

# Run tests
test:
    go test -race ./...

# Build all binaries to ./bin/
build:
    @mkdir -p bin
    go build -o bin/ ./cmd/...

# Run vulnerability scan
vuln:
    go tool govulncheck ./...

# Tidy modules and verify clean
tidy:
    go mod tidy
    git diff --exit-code go.mod go.sum
```

- [ ] **Step 2: Verify `just` lists recipes**

Run:
```bash
just
```
Expected: prints the list of recipes.

- [ ] **Step 3: Verify each Go recipe**

Run:
```bash
just fmt && just lint && just test && just build && just vuln
```
Expected: all exit 0. `bin/` directory now contains four binaries.

- [ ] **Step 4: Add `bin/` to `.gitignore` if not already covered**

`bin/` is already covered by `/bin/` in `.gitignore` (added in Task 1). Confirm by running:
```bash
git status
```
Expected: `bin/` does NOT appear.

- [ ] **Step 5: Commit**

```bash
git add justfile
git commit -m "chore: add justfile with go recipes (fmt/lint/test/build/vuln/tidy)"
```

---

## Task 5: Install lefthook hooks

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/lefthook.yml`

- [ ] **Step 1: Create `lefthook.yml`**

```yaml
pre-commit:
  parallel: false
  commands:
    gofumpt:
      glob: "*.go"
      run: go tool gofumpt -l -w {staged_files}
      stage_fixed: true
    goimports:
      glob: "*.go"
      run: go tool goimports -w {staged_files}
      stage_fixed: true
    golangci-lint:
      glob: "*.go"
      run: go tool golangci-lint run --new-from-rev=HEAD
    go-test:
      glob: "*.go"
      run: go test ./...

pre-push:
  commands:
    go-mod-tidy:
      run: |
        go mod tidy
        git diff --exit-code go.mod go.sum
```

- [ ] **Step 2: Install the git hooks**

Run from `/Users/lukasstrobl/Developer/fathom`:
```bash
lefthook install
```
Expected: prints "sync hooks" success messages. `.git/hooks/pre-commit` and `.git/hooks/pre-push` now exist.

- [ ] **Step 3: Verify hooks fire by appending a trailing newline to a tracked Go file**

```bash
echo "" >> cmd/base-collector/main.go
git add cmd/base-collector/main.go
git commit -m "test: smoke-test lefthook"
```
Expected: lefthook output appears showing `gofumpt`, `goimports`, `golangci-lint`, and `go-test` running in sequence. gofumpt collapses the extra newline; all four pass; commit lands.

- [ ] **Step 4: Roll back the smoke commit**

```bash
git reset --hard HEAD~1
```
Expected: commit removed, `main.go` restored to its pre-smoke state.

- [ ] **Step 5: Add `just hooks` recipe to the justfile**

Open `/Users/lukasstrobl/Developer/fathom/justfile` and append:

```just

# Install lefthook git hooks
hooks:
    lefthook install
```

- [ ] **Step 6: Commit lefthook config**

```bash
git add lefthook.yml justfile
git commit -m "chore: add lefthook pre-commit and pre-push hooks"
```

---

## Task 6: Create the Dockerfile (Go binaries + init-db stages)

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/Dockerfile`
- Create: `/Users/lukasstrobl/Developer/fathom/scripts/init-db.sh`

- [ ] **Step 1: Create `Dockerfile`**

```dockerfile
# syntax=docker/dockerfile:1.7

# ---- Goose binary (used by init-db stage) ----
FROM golang:1.24-alpine AS goose-installer
ENV CGO_ENABLED=0
RUN go install github.com/pressly/goose/v3/cmd/goose@latest

# ---- Build stage for Go binaries ----
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BINARY
RUN test -n "$BINARY" || (echo "BINARY build-arg is required" && exit 1)
ENV CGO_ENABLED=0
RUN go build -o /out/app ./cmd/${BINARY}

# ---- Runtime for Go binaries ----
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
COPY --from=builder /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

# ---- Init-db image (psql + goose + script) ----
FROM alpine:3.20 AS init-db
RUN apk add --no-cache postgresql-client bash
COPY --from=goose-installer /go/bin/goose /usr/local/bin/goose
COPY scripts/init-db.sh /init-db.sh
RUN chmod +x /init-db.sh
ENTRYPOINT ["/init-db.sh"]
```

- [ ] **Step 2: Create `scripts/init-db.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${DB_URL:?DB_URL is required}"

echo "init-db: running goose migrations"
goose -dir /migrations postgres "$DB_URL" up

echo "init-db: applying views"
shopt -s nullglob
for f in /views/*.sql; do
  echo "  applying $(basename "$f")"
  psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$f"
done

echo "init-db: done"
```

- [ ] **Step 3: Make the script executable on the host**

Run:
```bash
chmod +x scripts/init-db.sh
```

- [ ] **Step 4: Verify the Go binary stage builds**

Run:
```bash
docker build --build-arg BINARY=base-collector --target runtime -t fathom-base-collector:dev .
```
Expected: build succeeds. Final image is small (~few MB on distroless).

- [ ] **Step 5: Verify the init-db stage builds**

Run:
```bash
docker build --target init-db -t fathom-init-db:dev .
```
Expected: build succeeds.

- [ ] **Step 6: Commit**

```bash
git add Dockerfile scripts/init-db.sh
git commit -m "feat: add multi-stage Dockerfile (runtime + init-db) and init-db script"
```

---

## Task 7: Create docker-compose.yml and .env.example

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/docker-compose.yml`
- Create: `/Users/lukasstrobl/Developer/fathom/compose.override.yml.example`
- Create: `/Users/lukasstrobl/Developer/fathom/.env.example`
- Create: `/Users/lukasstrobl/Developer/fathom/migrations/.gitkeep`
- Create: `/Users/lukasstrobl/Developer/fathom/views/.gitkeep`

- [ ] **Step 1: Create empty `migrations/` and `views/` directories**

Run:
```bash
mkdir -p migrations views
touch migrations/.gitkeep views/.gitkeep
```

- [ ] **Step 2: Create `.env.example`**

```env
# Copy to .env and edit. .env is gitignored.

# Postgres
POSTGRES_USER=fathom
POSTGRES_PASSWORD=fathom
POSTGRES_DB=fathom
POSTGRES_PORT=5432

# Connection URL used by binaries and init-db (inside the compose network)
DB_URL=postgres://fathom:fathom@postgres:5432/fathom?sslmode=disable

# Host-side connection URL (used by `just migrate` from your terminal)
DB_URL_HOST=postgres://fathom:fathom@localhost:5432/fathom?sslmode=disable
```

- [ ] **Step 3: Create `docker-compose.yml`**

```yaml
name: fathom

services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
    ports:
      - "${POSTGRES_PORT}:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 2s
      timeout: 3s
      retries: 20

  init-db:
    build:
      context: .
      target: init-db
    environment:
      DB_URL: ${DB_URL}
    volumes:
      - ./migrations:/migrations:ro
      - ./views:/views:ro
    depends_on:
      postgres:
        condition: service_healthy
    restart: "no"

  base-collector:
    build:
      context: .
      target: runtime
      args:
        BINARY: base-collector
    image: fathom-base-collector:dev
    restart: unless-stopped
    environment:
      DB_URL: ${DB_URL}
    depends_on:
      init-db:
        condition: service_completed_successfully

  solana-collector:
    build:
      context: .
      target: runtime
      args:
        BINARY: solana-collector
    image: fathom-solana-collector:dev
    restart: unless-stopped
    environment:
      DB_URL: ${DB_URL}
    depends_on:
      init-db:
        condition: service_completed_successfully

  probe-collector:
    build:
      context: .
      target: runtime
      args:
        BINARY: probe-collector
    image: fathom-probe-collector:dev
    restart: unless-stopped
    environment:
      DB_URL: ${DB_URL}
    depends_on:
      init-db:
        condition: service_completed_successfully

  publisher:
    build:
      context: .
      target: runtime
      args:
        BINARY: publisher
    image: fathom-publisher:dev
    restart: "no"
    environment:
      DB_URL: ${DB_URL}
    depends_on:
      init-db:
        condition: service_completed_successfully
    profiles: ["manual"]

volumes:
  postgres-data:
```

- [ ] **Step 4: Create `compose.override.yml.example`**

```yaml
# Copy to compose.override.yml for local-only tweaks.
# Examples below — keep what you need, delete the rest.

services:
  base-collector:
    # Mount source for fast iteration without rebuild:
    # volumes:
    #   - .:/src:ro
    environment:
      LOG_LEVEL: debug

  postgres:
    # Expose on a different host port:
    # ports:
    #   - "5433:5432"
```

- [ ] **Step 5: Create the actual `.env` from the example for this session**

Run:
```bash
cp .env.example .env
```

(`.env` is gitignored — it won't be committed.)

- [ ] **Step 6: Validate compose file syntax**

Run:
```bash
docker compose config --quiet
```
Expected: exits 0 with no output.

- [ ] **Step 7: Bring up Postgres only and verify health**

Run:
```bash
docker compose up -d postgres
```
Expected: postgres service starts. Run:
```bash
docker compose ps
```
Expected: postgres status is `healthy`.

- [ ] **Step 8: Run init-db and verify it completes (no migrations or views yet — should still exit 0)**

Run:
```bash
docker compose run --rm init-db
```
Expected: prints "init-db: running goose migrations" → "goose: no migrations to run" → "init-db: applying views" → "init-db: done". Exit 0.

- [ ] **Step 9: Tear everything down**

Run:
```bash
docker compose down -v
```
Expected: services stopped, volume removed.

- [ ] **Step 10: Commit**

```bash
git add docker-compose.yml compose.override.yml.example .env.example migrations/.gitkeep views/.gitkeep
git commit -m "feat: add docker-compose with postgres, init-db, and four binary services"
```

---

## Task 8: Add Docker-related recipes to the justfile

**Files:**
- Modify: `/Users/lukasstrobl/Developer/fathom/justfile`

- [ ] **Step 1: Append Docker recipes to the justfile**

Open `/Users/lukasstrobl/Developer/fathom/justfile` and append:

```just

# --- Docker / compose ---

# Bring up postgres + init-db + collectors
up:
    docker compose up -d --build

# Stop everything (keep volume)
down:
    docker compose down

# Stop everything AND delete the postgres volume
nuke:
    docker compose down -v

# psql shell against the compose postgres
psql:
    docker compose exec postgres psql -U $POSTGRES_USER -d $POSTGRES_DB

# Re-run init-db (idempotent: goose only applies new migrations; views use CREATE OR REPLACE)
init-db:
    docker compose run --rm init-db

# Run a binary on demand: `just run publisher`
run binary:
    docker compose run --rm {{binary}}

# Tail logs for one or all services: `just logs` or `just logs base-collector`
logs *args:
    docker compose logs -f {{args}}

# --- Migrations (run from host against the exposed postgres port) ---

# Run goose against the compose postgres. Examples:
#   just migrate up
#   just migrate down
#   just migrate status
#   just migrate create add_payments_table sql
migrate *args:
    @set -a; source .env; set +a; \
        go tool goose -dir migrations postgres "$DB_URL_HOST" {{args}}

# Apply all view SQL files (idempotent)
apply-views:
    @set -a; source .env; set +a; \
        for f in views/*.sql; do \
            [ -e "$f" ] || break; \
            echo "applying $f"; \
            docker compose exec -T postgres psql "$DB_URL" -v ON_ERROR_STOP=1 -f - < "$f"; \
        done
```

- [ ] **Step 2: Add goose as a Go tool (for host-side `just migrate`)**

Run:
```bash
go get -tool github.com/pressly/goose/v3/cmd/goose@latest
```

- [ ] **Step 3: Verify `just up` brings everything up cleanly**

Run:
```bash
just up
```
Expected: builds images, starts postgres, runs init-db to completion, starts the three collector services. Run:
```bash
docker compose ps
```
Expected: `postgres` healthy; `base-collector`, `solana-collector`, `probe-collector` running (may be in restart loop because they exit after printing "starting" — that's fine for a stub).

- [ ] **Step 4: Verify `just init-db` reruns cleanly**

Run:
```bash
just init-db
```
Expected: same output as before, exits 0 (idempotent).

- [ ] **Step 5: Verify `just migrate status` works**

Run:
```bash
just migrate status
```
Expected: prints a table with no applied migrations (none exist yet).

- [ ] **Step 6: Verify `just psql` opens a shell (then exit)**

Run:
```bash
echo '\q' | just psql
```
Expected: prints psql banner, exits.

- [ ] **Step 7: Tear down**

Run:
```bash
just nuke
```

- [ ] **Step 8: Commit**

```bash
git add justfile go.mod go.sum
git commit -m "chore: add docker/compose and migration recipes to justfile"
```

---

## Task 9: Add a smoke migration to confirm goose works end-to-end

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/migrations/00001_init.sql`

- [ ] **Step 1: Create the migration**

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS _fathom_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    set_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO _fathom_meta (key, value) VALUES ('schema_initialized', 'true')
ON CONFLICT (key) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS _fathom_meta;
-- +goose StatementEnd
```

This is a real, useful row: it records that the schema has been touched. Future migrations add the actual `payments`, `services`, `probe_results` tables.

- [ ] **Step 2: Bring up postgres and run init-db**

Run:
```bash
just up
```
Expected: init-db applies migration `00001_init.sql`.

- [ ] **Step 3: Verify the table exists**

Run:
```bash
just psql -c "SELECT * FROM _fathom_meta;"
```
Expected: one row, `schema_initialized = true`.

Note: `just psql` recipe takes no args in the form written. Use this instead:
```bash
docker compose exec -T postgres psql -U fathom -d fathom -c "SELECT * FROM _fathom_meta;"
```
Expected: one row.

- [ ] **Step 4: Verify migrate status reports the migration applied**

Run:
```bash
just migrate status
```
Expected: shows `00001_init.sql` as Applied.

- [ ] **Step 5: Tear down**

Run:
```bash
just nuke
```

- [ ] **Step 6: Commit**

```bash
git add migrations/00001_init.sql
git commit -m "feat: add initial migration with _fathom_meta table"
```

---

## Task 10: Add a smoke view to confirm the view pipeline works

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/views/_smoke_v1.sql`

- [ ] **Step 1: Create the smoke view**

```sql
-- _smoke_v1: confirms the views pipeline (init-db applies *.sql in views/).
-- Delete this file when the first real view ships.
CREATE OR REPLACE VIEW _smoke_v1 AS
SELECT 'ok'::text AS status, now() AS computed_at;
```

- [ ] **Step 2: Bring up and verify**

Run:
```bash
just up
```
Expected: init-db applies the smoke view.

Verify:
```bash
docker compose exec -T postgres psql -U fathom -d fathom -c "SELECT * FROM _smoke_v1;"
```
Expected: one row with status='ok'.

- [ ] **Step 3: Verify `just apply-views` re-applies idempotently**

Run:
```bash
just apply-views
```
Expected: prints `applying views/_smoke_v1.sql`, exits 0 with no error (CREATE OR REPLACE is idempotent).

- [ ] **Step 4: Tear down**

Run:
```bash
just nuke
```

- [ ] **Step 5: Commit**

```bash
git add views/_smoke_v1.sql
git commit -m "feat: add _smoke_v1 view to confirm views pipeline"
```

---

## Task 11: GitHub Actions CI

**Files:**
- Create: `/Users/lukasstrobl/Developer/fathom/.github/workflows/ci.yml`

- [ ] **Step 1: Create the workflow file**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: golangci-lint
        run: go tool golangci-lint run ./...

  test:
    name: Test
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: fathom
          POSTGRES_PASSWORD: fathom
          POSTGRES_DB: fathom
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U fathom"
          --health-interval 2s
          --health-timeout 3s
          --health-retries 20
    env:
      DB_URL_HOST: postgres://fathom:fathom@localhost:5432/fathom?sslmode=disable
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: Test
        run: go test -race ./...

  vuln:
    name: Vuln scan
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - name: govulncheck
        run: go tool govulncheck ./...

  build:
    name: Docker build (${{ matrix.binary }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        binary: [base-collector, solana-collector, probe-collector, publisher]
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - name: Build image
        run: |
          docker build \
            --build-arg BINARY=${{ matrix.binary }} \
            --target runtime \
            -t fathom-${{ matrix.binary }}:ci \
            .
```

- [ ] **Step 2: Validate workflow syntax**

GitHub Actions has no perfect local validator. The next-best check is to ensure YAML parses and the file is well-formed:

```bash
docker run --rm -v "$PWD/.github/workflows":/workflows mikefarah/yq:latest \
    'true' /workflows/ci.yml
```
Expected: exits 0 (YAML is valid).

If `mikefarah/yq` pull is undesirable, fallback:
```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))"
```
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add lint/test/vuln/docker-build workflow"
```

---

## Task 12: Final verification of all success criteria

**Files:** none (verification only)

- [ ] **Step 1: Confirm `just up` works end-to-end**

```bash
just up
docker compose ps
```
Expected: postgres healthy; init-db exited 0; three collectors running (or crash-looping after their stub `Println` — that's the current expected behavior for stubs).

- [ ] **Step 2: Confirm `just lint && just test` exit 0**

```bash
just lint && just test
echo "exit: $?"
```
Expected: `exit: 0`.

- [ ] **Step 3: Confirm `just build` produces four binaries**

```bash
just build
ls bin/
```
Expected: four files: `base-collector`, `solana-collector`, `probe-collector`, `publisher`.

- [ ] **Step 4: Confirm hooks fire on commit**

Append a benign trailing newline to a tracked Go file and commit:
```bash
echo "" >> cmd/publisher/main.go
git add cmd/publisher/main.go
git commit -m "test: verify hooks fire"
```
Expected: lefthook output appears showing gofumpt/goimports/golangci-lint/go-test all running. Commit succeeds.

Cleanup:
```bash
git reset --hard HEAD~1
```

- [ ] **Step 5: Confirm `just nuke` cleans up**

```bash
just nuke
docker compose ps
```
Expected: no services listed.

- [ ] **Step 6: Final commit (only if anything residual changed)**

If `git status` is clean, skip. Otherwise stage and commit anything missed:
```bash
git status
```

---

## Done criteria (from the spec)

After this plan executes:

- [x] `just up` brings up Postgres, runs `init-db` to completion, leaves collectors running.
- [x] `just lint && just test` exit 0.
- [x] `just build` produces four binaries.
- [x] `git commit` triggers gofumpt/goimports/golangci-lint/test via lefthook.
- [x] `.github/workflows/ci.yml` runs lint + test + vuln + matrix-build on PR/push.

Anything beyond these — real RPC clients, actual `payments`/`services` tables, the publisher's git-push code — is out of scope for this plan and belongs in a separate spec.
