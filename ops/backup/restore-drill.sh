#!/usr/bin/env bash
# Monthly restore drill: "a backup you have never restored is a guess."
# Downloads the newest database backup, decrypts it with your age identity,
# restores it into a scratch database, prints row counts, then drops it.
#   ops/backup/restore-drill.sh ~/.config/careerglobe/backup-key.txt
# Run it on the server or the Mac (needs psql, pg_restore, age, rclone).
set -euo pipefail

identity="${1:?usage: restore-drill.sh <age identity file>}"
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ -f "$REPO/.env" ]]; then set -a; . "$REPO/.env"; set +a; fi
: "${DATABASE_URL:?}" "${R2_ACCOUNT_ID:?}" "${R2_ACCESS_KEY_ID:?}" "${R2_SECRET_ACCESS_KEY:?}" "${R2_BUCKET:?}"

export RCLONE_CONFIG_R2_TYPE=s3 RCLONE_CONFIG_R2_PROVIDER=Cloudflare \
  RCLONE_CONFIG_R2_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
  RCLONE_CONFIG_R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com"

latest="$(rclone lsf "r2:$R2_BUCKET/daily/" --include 'db-*.dump.age' | sort | tail -1)"
[[ -n "$latest" ]] || { echo "no backups found"; exit 1; }
work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
rclone copy "r2:$R2_BUCKET/daily/$latest" "$work/"
age -d -i "$identity" "$work/$latest" > "$work/db.dump"

scratch=careerglobe_restore_drill
psql "$DATABASE_URL" -qc "DROP DATABASE IF EXISTS $scratch" -c "CREATE DATABASE $scratch"
target="$(python3 -c 'import sys,urllib.parse as u;p=u.urlsplit(sys.argv[1]);print(u.urlunsplit(p._replace(path="/'"$scratch"'")))' "$DATABASE_URL")"
psql "$target" -qc "CREATE EXTENSION IF NOT EXISTS postgis"
pg_restore --no-owner --dbname="$target" "$work/db.dump" 2>&1 | grep -v 'already exists' || true
echo "restored $latest into $scratch:"
psql "$target" -c "SELECT 'companies' t, count(*) FROM companies UNION ALL SELECT 'jobs', count(*) FROM jobs
  UNION ALL SELECT 'users', count(*) FROM users UNION ALL SELECT 'crawl_runs', count(*) FROM crawl_runs"
psql "$DATABASE_URL" -qc "DROP DATABASE $scratch"
echo "drill OK: dropped $scratch"
