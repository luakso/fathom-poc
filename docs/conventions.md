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
