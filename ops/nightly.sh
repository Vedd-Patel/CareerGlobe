#!/usr/bin/env bash
# The nightly job, the same on the Mac (launchd) and the server (systemd):
# crawl every company, then classify jobs and score them for every user.
# Each step pings its own heartbeat URL from .env when that URL is set.
#   ops/nightly.sh                 full nightly run
#   ops/nightly.sh --if-triggered  only if the GitHub watchdog asked for a re-run
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO"
if [[ -f .env ]]; then
  set -a; . ./.env; set +a
fi
PY="${CRAWLER_PYTHON:-$REPO/crawler/.venv/bin/python}"
NIGHTLY="$("$REPO/ops/build-go.sh" nightly)"

if [[ "${1:-}" == "--if-triggered" ]]; then
  status=0
  "$PY" crawler/crawl.py --if-triggered || status=$?
  if [[ $status -eq 3 ]]; then exit 0; fi   # nothing was requested
  [[ $status -eq 0 ]] || exit $status
else
  # Companies left over when last night hit the 05:30 budget go first: sources
  # are ordered by their last successful crawl.
  "$PY" crawler/crawl.py
fi

"$NIGHTLY" run
