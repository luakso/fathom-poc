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

# Install lefthook git hooks
hooks:
    lefthook install
