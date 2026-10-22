#!/usr/bin/env bash
# Nightly backup (spec: Backups that actually restore):
#   pg_dump of the database and a tar of the (already encrypted) resume files,
#   each encrypted with age to BACKUP_AGE_RECIPIENT, uploaded to Cloudflare R2.
#   Keeps 7 daily and 4 weekly (Sunday) copies. Pings HEALTHCHECK_BACKUP_URL.
# Needs: pg_dump, age, rclone. Credentials come from .env (R2_*), never a config file.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
if [[ -f "$REPO/.env" ]]; then set -a; . "$REPO/.env"; set +a; fi
: "${BACKUP_DATABASE_URL:?}" "${BACKUP_AGE_RECIPIENT:?}" "${R2_ACCOUNT_ID:?}" "${R2_ACCESS_KEY_ID:?}" \
  "${R2_SECRET_ACCESS_KEY:?}" "${R2_BUCKET:?}"

ping() { [[ -n "${HEALTHCHECK_BACKUP_URL:-}" ]] && curl -fsS -m 10 --retry 3 -o /dev/null --data-raw "${2:-}" "${HEALTHCHECK_BACKUP_URL%/}$1" || true; }
ping /start
trap 'ping /fail "backup failed at line $LINENO"' ERR

export RCLONE_CONFIG_R2_TYPE=s3 RCLONE_CONFIG_R2_PROVIDER=Cloudflare \
  RCLONE_CONFIG_R2_ACCESS_KEY_ID="$R2_ACCESS_KEY_ID" RCLONE_CONFIG_R2_SECRET_ACCESS_KEY="$R2_SECRET_ACCESS_KEY" \
  RCLONE_CONFIG_R2_ENDPOINT="https://${R2_ACCOUNT_ID}.r2.cloudflarestorage.com" RCLONE_CONFIG_R2_ACL=private

work="$(mktemp -d /var/lib/careerglobe/backup.XXXXXX 2>/dev/null || mktemp -d)"
trap 'rm -rf "$work"' EXIT
stamp="$(date -u +%Y-%m-%d)"
kind=daily
[[ "$(date +%u)" == "7" ]] && kind=weekly

pg_dump --format=custom --no-owner --dbname="$BACKUP_DATABASE_URL" | age -r "$BACKUP_AGE_RECIPIENT" > "$work/db-$stamp.dump.age"
if [[ -n "${RESUME_STORAGE_DIR:-}" && -d "$RESUME_STORAGE_DIR" ]]; then
  tar -C "$RESUME_STORAGE_DIR" -cf - . | age -r "$BACKUP_AGE_RECIPIENT" > "$work/resumes-$stamp.tar.age"
fi

rclone copy --s3-no-check-bucket "$work" "r2:$R2_BUCKET/$kind/"
# Retention: 7 daily, 4 weekly.
rclone delete --min-age 7d "r2:$R2_BUCKET/daily/"
rclone delete --min-age 29d "r2:$R2_BUCKET/weekly/"

size="$(du -sh "$work" | cut -f1)"
ping "" "uploaded $kind backup $stamp ($size)"
echo "backup $kind $stamp ($size) uploaded"
