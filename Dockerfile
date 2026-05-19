# syntax=docker/dockerfile:1.7

# ---- Goose binary (used by init-db stage) ----
FROM golang:1.26-alpine AS goose-installer
ENV CGO_ENABLED=0
ARG GOOSE_VERSION=v3.27.1
RUN go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION}

# ---- Build stage for Go binaries ----
FROM golang:1.26-alpine AS builder
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
