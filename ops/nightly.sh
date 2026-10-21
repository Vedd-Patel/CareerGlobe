#!/usr/bin/env bash
# The nightly job, the same on the Mac (launchd) and the server (systemd):
# crawl every company, then (from Phase 4) match and send the morning push.
# Each step pings its own heartbeat URL from .env when that URL is set.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO"
if [[ -f .env ]]; then
  set -a; . ./.env; set +a
fi
PY="${CRAWLER_PYTHON:-$REPO/crawler/.venv/bin/python}"

# Companies left over when last night hit the 05:30 budget go first: sources
# are ordered by their last successful crawl.
"$PY" crawler/crawl.py

if [[ -f crawler/match.py ]]; then
  "$PY" crawler/match.py
fi
