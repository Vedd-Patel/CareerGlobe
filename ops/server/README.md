# Server: build or rebuild in an evening

One Oracle Cloud Always Free ARM VM (2 OCPU / 12 GB, Ubuntu 24.04, Mumbai or Hyderabad
region) runs PostgreSQL + PostGIS, the Go API, the nightly crawl and matching. Nothing is
reachable from the internet except SSH; the API is published through a Cloudflare Tunnel.

## 1. Create the VM
- Oracle Cloud → Compute → Instance, shape `VM.Standard.A1.Flex`, 2 OCPU, 12 GB, Ubuntu 24.04.
- Add your SSH public key. In the VCN security list allow only TCP 22 in.

## 2. Install
```bash
ssh ubuntu@<vm-ip>
sudo git clone https://github.com/Vedd-Patel/CareerGlobe /opt/careerglobe
sudo nano /opt/careerglobe/.env      # paste the server .env (see .env.example), chmod 600
sudo /opt/careerglobe/ops/server/setup.sh
```
Server `.env` differences from the laptop: `DATABASE_URL` points at the local Postgres
(owner role, used for migrations); `CRAWLER_DATABASE_URL`, `API_DATABASE_URL`,
`BACKUP_DATABASE_URL` use the three roles below; `RESUME_STORAGE_DIR=/var/lib/careerglobe/resumes`;
new random values for `ACCESS_CODE_PEPPER`, `RESUME_ENCRYPTION_KEY` (`openssl rand -base64 32`)
and `OPS_TOKEN` (`openssl rand -hex 24`). **Keep a copy of the pepper and the resume key in
your password manager: without them codes and resumes cannot be recovered from a backup.**

## 3. Database
```bash
sudo -u postgres createuser --pwprompt careerglobe        # the owner (DATABASE_URL)
sudo -u postgres createdb -O careerglobe careerglobe
sudo -u postgres psql careerglobe -c 'CREATE EXTENSION postgis'
cd /opt/careerglobe && set -a && . ./.env && set +a
api/bin/goose -dir db/migrations postgres "$DATABASE_URL" up
psql "$DATABASE_URL" -v crawler_pw="'…'" -v api_pw="'…'" -v backup_pw="'…'" -f ops/server/db-roles.sql
```
Restoring instead of starting fresh: create the empty database and extension, then
`pg_restore --no-owner -d "$DATABASE_URL" db.dump` (decrypt with `age -d -i key.txt`), then run
`db-roles.sql` again.

## 4. Tunnel
Follow `ops/cloudflared/config.yml.example`. Check from your laptop:
`curl https://api.yourdomain.in/healthz` → `{"status":"ok"}`.

## 5. Start everything
```bash
sudo systemctl enable --now careerglobe-api careerglobe-nightly.timer careerglobe-push.timer careerglobe-backup.timer
systemctl list-timers 'careerglobe-*'
journalctl -u careerglobe-api -f
```

## 6. Backups (Cloudflare R2, free tier)
- Create a bucket and an R2 API token (Object Read & Write, that bucket only); put
  `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET` in `.env`.
- On your laptop, `age-keygen -o backup-key.txt`; put the **public** key (`age1…`) in
  `BACKUP_AGE_RECIPIENT`. Keep `backup-key.txt` off the server, in your password manager.
- Once a month: `ops/backup/restore-drill.sh backup-key.txt` (prints row counts).

## 7. Monitoring
- healthchecks.io: checks "crawl", "match", "backup" → `HEALTHCHECK_*_URL` in `.env`.
- GitHub repository secrets `API_URL` and `OPS_TOKEN` turn on `.github/workflows/watchdog.yml`.
- Oracle reclaims idle Always Free VMs; the nightly crawl keeps it busy, but read Oracle's emails.

## Updating
```bash
cd /opt/careerglobe && sudo -u careerglobe git pull
sudo -u careerglobe bash -c 'set -a; . ./.env; set +a; api/bin/goose -dir db/migrations postgres "$DATABASE_URL" up'
sudo -u careerglobe bash -c 'cd api && go build -o bin/api ./cmd/api && go build -o bin/nightly ./cmd/nightly'
sudo systemctl restart careerglobe-api
```
