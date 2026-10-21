#!/usr/bin/env bash
# launchd entry point on the Mac: keeps the Mac awake for the whole run
# (caffeinate -s needs AC power) and writes one log file per night.
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOGS="$HOME/Library/Logs/CareerGlobe"
mkdir -p "$LOGS"
LOG="$LOGS/nightly-$(date +%Y-%m-%d).log"
find "$LOGS" -name 'nightly-*.log' -mtime +14 -delete 2>/dev/null || true

# Docker Desktop must be running for the local Postgres.
if ! /usr/local/bin/docker info >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
  open -a Docker
  for _ in $(seq 1 90); do docker info >/dev/null 2>&1 && break; sleep 2; done
fi
(cd "$REPO" && docker compose up -d db) >>"$LOG" 2>&1

echo "=== $(date) nightly start" >>"$LOG"
/usr/bin/caffeinate -i -s "$REPO/ops/nightly.sh" >>"$LOG" 2>&1
status=$?
echo "=== $(date) nightly end, exit $status" >>"$LOG"
exit $status
