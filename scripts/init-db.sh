#!/usr/bin/env bash
set -euo pipefail

: "${DB_URL:?DB_URL is required}"

echo "init-db: running goose migrations"
# goose exits 1 when no .sql files exist; skip if dir is empty
if ls /migrations/*.sql 1>/dev/null 2>&1; then
  goose -dir /migrations postgres "$DB_URL" up
else
  echo "  no migration files found, skipping"
fi

echo "init-db: applying views"
shopt -s nullglob
for f in /views/*.sql; do
  echo "  applying $(basename "$f")"
  psql "$DB_URL" -v ON_ERROR_STOP=1 -f "$f"
done

echo "init-db: done"
