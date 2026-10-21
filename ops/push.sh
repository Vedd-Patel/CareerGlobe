#!/usr/bin/env bash
# Every 15 minutes: send the morning push to users whose chosen time has come
# (at most once a day each), then pick up a crawl re-run the watchdog asked for.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO"
if [[ -f .env ]]; then
  set -a; . ./.env; set +a
fi
"$("$REPO/ops/build-go.sh" nightly)" push
"$REPO/ops/nightly.sh" --if-triggered
