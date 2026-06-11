# Fathom Production Deployment

Target box: **`picasso@reef`** (Ubuntu, single VPS). Login/deploy user `picasso`.
Stack: Docker Compose (`docker-compose.prod.yml`) — tuned Postgres (no exposed
port), on-demand collectors/publisher pulled from GHCR, Caddy serving static JSON.

Deploys are **manual and deliberate**: CI pushes images to GHCR on merge to `main`;
you run `./deploy.sh` here when you choose to ship.

---

## Phase A — Host provisioning (one-time, sudo)

1. Harden the existing `picasso` user: SSH key-only auth (`PasswordAuthentication no`
   in `/etc/ssh/sshd_config`), then `sudo systemctl restart ssh`.
2. Firewall — allow only SSH + web:
   ```bash
   sudo ufw allow 22/tcp && sudo ufw allow 80/tcp && sudo ufw allow 443/tcp
   sudo ufw enable
   ```
3. Unattended security updates:
   ```bash
   sudo apt-get update && sudo apt-get install -y unattended-upgrades
   sudo dpkg-reconfigure -plow unattended-upgrades
   ```
4. Install Docker Engine + Compose plugin (official repo — NOT Docker Desktop):
   ```bash
   sudo apt-get install -y ca-certificates curl
   sudo install -m 0755 -d /etc/apt/keyrings
   sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
   sudo chmod a+r /etc/apt/keyrings/docker.asc
   echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo $VERSION_CODENAME) stable" | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
   sudo apt-get update
   sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
   ```
5. Let `picasso` run Docker without sudo, then re-login:
   ```bash
   sudo usermod -aG docker picasso
   ```
6. Disk-usage alarm (the `temp_file_limit` in compose caps per-query temp files;
   this catches everything else). Create `/etc/cron.hourly/disk-alarm`:
   ```bash
   #!/usr/bin/env bash
   THRESH=85
   USE=$(df --output=pcent / | tail -1 | tr -dc '0-9')
   [ "$USE" -ge "$THRESH" ] && logger -t disk-alarm "root fs at ${USE}% (>= ${THRESH}%)"
   ```
   ```bash
   sudo chmod +x /etc/cron.hourly/disk-alarm
   ```

## Phase B — App bootstrap (as `picasso`)

7. Clone the repo:
   ```bash
   sudo mkdir -p /opt/fathom && sudo chown picasso:picasso /opt/fathom
   git clone https://github.com/luakso/fathom.git /opt/fathom
   cd /opt/fathom
   ```
8. Log in to GHCR (needs a GitHub PAT with `read:packages`):
   ```bash
   echo "<YOUR_PAT>" | docker login ghcr.io -u luakso --password-stdin
   ```
9. Create the prod env file from the template and fill real values:
   ```bash
   cp .env.prod.example .env.prod
   # edit .env.prod: strong POSTGRES_PASSWORD (and matching DB_URL),
   # BASE_HYPERSYNC_BEARER_TOKEN if you have one, tuning to box RAM.
   chmod 600 .env.prod
   ```
10. Bring up Postgres and apply all migrations + views to the empty DB
    (fast — empty table, no 64M-row rewrite):
    ```bash
    docker compose --env-file .env.prod -f docker-compose.prod.yml up -d postgres
    docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm init-db
    ```

## Phase C — Seed data (re-backfill from scratch)

> Long-running. Run inside `tmux` so a dropped SSH session does not kill it.
> Pick block ranges for the window you want to index.

11. Base backfill:
    ```bash
    tmux new -s backfill
    docker compose --env-file .env.prod -f docker-compose.prod.yml \
      run --rm base-collector backfill --from-block <X> --to-block <Y>
    ```
12. Solana backfill — **not yet implemented.** The `solana-collector` binary is
    currently a stub: it connects to the DB, logs `solana-collector ready`, and
    exits without indexing (it ignores `backfill` args). Running it appears to
    succeed but writes nothing. Skip this step until the collector's ingest loop
    lands; v1 data is Base-only.
13. Recompute the rollup cube:
    ```bash
    docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm publisher rollup
    ```
14. Emit static JSON into the shared `dist` volume:
    ```bash
    docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm publisher emit --out /dist
    ```

## Phase D — Serve

15. (When you have a domain) point a DNS A-record at `reef`'s public IP and set
    `FATHOM_DOMAIN=<domain>` in `.env.prod`. Until then leave `FATHOM_DOMAIN=:80`.
16. Start Caddy:
    ```bash
    docker compose --env-file .env.prod -f docker-compose.prod.yml up -d caddy
    ```
17. Verify:
    ```bash
    curl -sS http://localhost/economy.json | head
    # with a domain + HTTPS: curl -sS https://<domain>/economy.json | head
    ```

## Phase E — Ongoing operations (manual)

**Refresh data** (re-run whenever you want fresh numbers):
```bash
cd /opt/fathom
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm base-collector backfill --from-block <X> --to-block <Y>
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm publisher rollup
docker compose --env-file .env.prod -f docker-compose.prod.yml run --rm publisher emit --out /dist
```

**Deploy new code** (after merging to `main` and CI has pushed images):
```bash
cd /opt/fathom
./deploy.sh                      # latest
FATHOM_TAG=<git-sha> ./deploy.sh # pin / roll back to a specific build
```

**Back up Postgres** (copy off-box):
```bash
docker compose --env-file .env.prod -f docker-compose.prod.yml exec -T postgres \
  pg_dump -U fathom -d fathom --format=custom > "fathom-$(date +%F).dump"
# then scp/rsync the .dump to another machine or object storage.
```
