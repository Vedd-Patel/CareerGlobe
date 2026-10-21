# Operations

Everything that runs the crawl (and later matching and push) every night.

| File | What it is |
|---|---|
| `nightly.sh` | The nightly job: crawl every company, then `nightly run` (classify jobs, score every user, saved-company alerts, reminders). Same script on the Mac and the server. |
| `push.sh` | Every 15 minutes: `nightly push` (morning push at each user's chosen time) and a crawl re-run if the GitHub watchdog asked for one. |
| `build-go.sh` | Builds `api/bin/<name>` from `api/cmd/<name>` when the Go sources changed (needs Go installed). |
| `mac/run-nightly.sh` | Mac entry point: starts Docker's Postgres, keeps the Mac awake with `caffeinate`, writes `~/Library/Logs/CareerGlobe/nightly-YYYY-MM-DD.log`. |
| `mac/com.careerglobe.nightly.plist.template` | launchd job: 01:30 every night. |
| `mac/install.sh` | Installs / removes both launchd jobs (nightly at 01:30, push every 15 minutes). |
| `systemd/careerglobe-nightly.{service,timer}` | The nightly job on the Oracle server (01:30 IST, `MemoryMax=6G`). |
| `systemd/careerglobe-push.{service,timer}` | The 15-minute push / re-run job on the server. |
| `../.github/workflows/watchdog.yml` | GitHub Actions, 08:00 IST: checks `/healthz` and that last night's crawl finished; asks for a re-run if not. |

## Nightly run on the Mac (until the server exists)

1. Plug the Mac in (`caffeinate -s` only holds the Mac awake on AC power) and set its
   time zone to India (System Settings → General → Date & Time), since launchd uses local time.
2. Install the job:
   ```bash
   ops/mac/install.sh
   ```
3. Wake the Mac at 01:25 every day (asks for your password once):
   ```bash
   sudo pmset repeat wakeorpoweron MTWRFSU 01:25:00
   pmset -g sched          # check it
   ```
4. Try it now instead of waiting for 01:30:
   ```bash
   launchctl kickstart -k gui/$(id -u)/com.careerglobe.nightly
   tail -f ~/Library/Logs/CareerGlobe/nightly-$(date +%Y-%m-%d).log
   ```

**macOS privacy check.** The repo lives in `~/Desktop`, which macOS protects. If the log
says `Operation not permitted`, either move the repo out of Desktop/Documents/Downloads
(for example to `~/code/CareerGlobe`, then run `ops/mac/install.sh` again), or give
`/bin/bash` Full Disk Access in System Settings → Privacy & Security.

If the Mac was asleep at 01:30, launchd runs the job as soon as it wakes. To stop:
`ops/mac/install.sh --remove` and `sudo pmset repeat cancel`.

## Access codes from the command line

```bash
set -a; . ./.env; set +a
"$(ops/build-go.sh nightly)" codes -label "Ravi" -uses 1 -days 30
```

The code is printed once; only its HMAC is stored. The admin console (S30) does the same.

## Heartbeat (healthchecks.io)

Free account → new check named "crawl", schedule "daily", grace time 6 hours → copy its
ping URL into `.env`:

```
HEALTHCHECK_CRAWL_URL=https://hc-ping.com/your-uuid
```

Full and resumed runs ping `/start` when they begin, the plain URL when the night is good,
and `/fail` (with the summary as the body) when more than 10% of companies failed, the
05:30 budget cut the run short, or the crawl crashed. Empty URL: no pings. `--only` runs
never ping. Make a second check "match" for `HEALTHCHECK_MATCH_URL` (pinged by `nightly run`).

## GitHub watchdog

Once the API is reachable through the tunnel, add two repository secrets (Settings →
Secrets and variables → Actions): `API_URL` (for example `https://api.yourdomain.in`) and
`OPS_TOKEN` (the same random value as in the server's `.env`). Until then the workflow
does nothing. Every push to the repo keeps the scheduled workflow from being auto-disabled.

## Server (later)

See `server/README.md` once Phase 0 server work is done. In short: copy the repo to
`/opt/careerglobe`, put `.env` next to it (chmod 600), then

```bash
sudo cp ops/systemd/careerglobe-*.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now careerglobe-nightly.timer careerglobe-push.timer
systemctl list-timers 'careerglobe-*' 
```
