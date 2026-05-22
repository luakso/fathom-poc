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

# Run integration tests (requires Docker daemon for testcontainers)
test-integration:
    go test -tags=integration -race -v ./...

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

# Install lefthook git hooks
hooks:
    lefthook install

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
    @set -a; . ./.env; set +a; \
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
    @set -a; . ./.env; set +a; \
        go tool goose -dir database/migrations postgres "$DB_URL_HOST" {{args}}

# Apply all view SQL files (idempotent)
apply-views:
    @set -a; . ./.env; set +a; \
        for f in database/views/*.sql; do \
            [ -e "$f" ] || break; \
            echo "applying $f"; \
            docker compose exec -T postgres psql "$DB_URL" -v ON_ERROR_STOP=1 -f - < "$f"; \
        done
