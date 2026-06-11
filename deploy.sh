#!/usr/bin/env bash
# Fathom production deploy.
#
# Usage:
#   ./deploy.sh                    # deploy the :latest images from GHCR
#   FATHOM_TAG=<git-sha> ./deploy.sh   # deploy / roll back to a specific image tag
#
# Prereqs (one-time): docker login ghcr.io ; a populated ./.env.prod (chmod 600).
set -euo pipefail

REPO_DIR="${REPO_DIR:-/opt/fathom}"
COMPOSE_FILE="docker-compose.prod.yml"
cd "$REPO_DIR"

# 1. Sync repo: compose file, migrations, views, and Caddyfile all live here.
git fetch --quiet origin
git checkout main
git pull --ff-only

# 2. Load prod env (POSTGRES_*, DB_URL, FATHOM_TAG, FATHOM_DOMAIN, tuning).
set -a
# shellcheck disable=SC1091
. ./.env.prod
set +a
export FATHOM_TAG="${FATHOM_TAG:-latest}"

echo ">> Deploying FATHOM_TAG=$FATHOM_TAG"

# 3. Pull target images from GHCR (requires prior docker login ghcr.io).
docker compose -f "$COMPOSE_FILE" pull \
  postgres init-db base-collector solana-collector probe-collector publisher caddy

# 4. Bring Postgres up, then apply migrations + views (idempotent: goose only
#    applies new migrations; views use CREATE OR REPLACE).
docker compose -f "$COMPOSE_FILE" up -d postgres
docker compose -f "$COMPOSE_FILE" run --rm init-db

# 5. (Re)start the only long-running services. Collectors + publisher stay
#    on-demand (docker compose -f docker-compose.prod.yml run --rm <svc> ...).
docker compose -f "$COMPOSE_FILE" up -d postgres caddy

echo ">> Deploy complete (FATHOM_TAG=$FATHOM_TAG)."
