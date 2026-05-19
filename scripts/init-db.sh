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
