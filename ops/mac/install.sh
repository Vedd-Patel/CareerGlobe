#!/usr/bin/env bash
# Install (or reinstall) the nightly LaunchAgent for this checkout.
#   ops/mac/install.sh            install and load
#   ops/mac/install.sh --remove   unload and delete
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LABEL=com.careerglobe.nightly
PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"

launchctl bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
if [[ "${1:-}" == "--remove" ]]; then
  rm -f "$PLIST"
  echo "removed $LABEL"
  exit 0
fi

mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/CareerGlobe"
sed -e "s#__REPO__#$REPO#g" -e "s#__HOME__#$HOME#g" "$REPO/ops/mac/$LABEL.plist.template" > "$PLIST"
plutil -lint "$PLIST" >/dev/null
launchctl bootstrap "gui/$(id -u)" "$PLIST"
echo "installed $LABEL -> runs $REPO/ops/mac/run-nightly.sh at 01:30"
echo
echo "Wake the Mac a few minutes before (needs your password, run once):"
echo "  sudo pmset repeat wakeorpoweron MTWRFSU 01:25:00"
echo "Test now:  launchctl kickstart -k gui/$(id -u)/$LABEL ; tail -f ~/Library/Logs/CareerGlobe/nightly-\$(date +%Y-%m-%d).log"
