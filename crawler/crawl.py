"""Nightly crawl of every careers source.

    python crawl.py --seeds ../seeds/companies.csv   # full run, start to finish
    python crawl.py --only "Groww"                   # one company, for testing
    python crawl.py --resume                         # continue a run that stopped
"""

import argparse
import asyncio
import logging
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

import config
import seeds
import store
from adapters import ADAPTERS, ParseError
from fetcher import FetchError, PoliteClient, RobotsDisallowed
from jobdiff import diff, diff_not_modified
from urls import job_key

log = logging.getLogger("crawl")
IST = ZoneInfo("Asia/Kolkata")
DEFAULT_SEEDS = config.REPO_ROOT / "seeds" / "companies.csv"
FAILED_CSV = config.REPO_ROOT / "failed.csv"


@dataclass
class Tally:
    ok: int = 0
    failed: int = 0
    new: int = 0
    reopened: int = 0
    closed: int = 0
    not_started: int = 0
    failures: list[str] = field(default_factory=list)


def stop_time(start: datetime, hhmm: str) -> datetime:
    """The first CRAWLER_STOP_AT_IST after the run started."""
    h, m = map(int, hhmm.split(":"))
    local = start.astimezone(IST)
    stop = local.replace(hour=h, minute=m, second=0, microsecond=0)
    return stop if stop > local else stop + timedelta(days=1)


async def crawl_source(conn, db_lock: asyncio.Lock, client: PoliteClient, run, src: store.Source,
                       tally: Tally) -> None:
    t0 = time.monotonic()
    adapter = ADAPTERS.get(src.ats_type or "")
    if adapter is None or not src.ats_slug:
        reason = f"no adapter for ATS {src.ats_type!r} (custom page: needs Phase 2 sources)"
        return await _fail(conn, db_lock, run, src, reason, tally, t0)

    try:
        res = await client.get(adapter.board_url(src.ats_slug), etag=src.etag, last_modified=src.last_modified)
    except RobotsDisallowed as e:
        return await _fail(conn, db_lock, run, src, str(e), tally, t0, blocked=True)
    except FetchError as e:
        return await _fail(conn, db_lock, run, src, str(e), tally, t0)

    if res.not_modified:
        compute, count = diff_not_modified, "unchanged (304)"
    else:
        try:
            jobs = adapter.parse(res.json(), src.ats_slug)
        except (ParseError, ValueError) as e:
            return await _fail(conn, db_lock, run, src, f"unexpected response: {e}", tally, t0)
        fetched = [(job_key(src.company_id, src.ats_type, j.source_job_id, j.apply_url), j) for j in jobs]
        compute, count = (lambda existing: diff(existing, fetched)), f"{len(jobs)} jobs"

    async with db_lock:
        d = await store.apply_success(conn, run, src, compute, res.etag, res.last_modified)
    secs = time.monotonic() - t0
    if d.broken:
        tally.failed += 1
        tally.failures.append(f"{src.company}: BROKEN {d.broken_reason}")
        log.warning("BROKEN %-28s %s  %.1fs", src.company, d.broken_reason, secs)
        return
    tally.ok += 1
    tally.new += len(d.inserts)
    tally.reopened += len(d.reopens)
    tally.closed += len(d.closes)
    log.info("OK     %-28s %-10s %s  +%d new  %d reopened  %d closed  %.1fs", src.company, src.ats_type,
             count, len(d.inserts), len(d.reopens), len(d.closes), secs)


async def _fail(conn, db_lock, run, src, reason, tally, t0, blocked=False):
    async with db_lock:
        await store.record_failure(conn, run, src, reason, blocked=blocked)
    tally.failed += 1
    tally.failures.append(f"{src.company}: {reason}")
    log.warning("FAILED %-28s %s  %.1fs", src.company, reason, time.monotonic() - t0)


async def crawl(conn, client: PoliteClient, settings: config.Settings, run, sources: list[store.Source],
                deadline: datetime | None) -> Tally:
    tally = Tally()
    slots = asyncio.Semaphore(settings.max_hosts_in_flight)
    # One shared DB connection: fetches run in parallel, transactions one at a time.
    db_lock = asyncio.Lock()

    async def one(src):
        async with slots:
            if deadline and datetime.now(IST) >= deadline:
                tally.not_started += 1
                return
            await crawl_source(conn, db_lock, client, run, src, tally)

    await asyncio.gather(*(one(s) for s in sources))
    return tally


async def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    mode = p.add_mutually_exclusive_group()
    mode.add_argument("--only", metavar="NAME", help="crawl one company by name (not recorded in crawl_runs)")
    mode.add_argument("--resume", action="store_true", help="continue the latest unfinished run")
    p.add_argument("--seeds", type=Path, default=DEFAULT_SEEDS, help="seed CSV (default: seeds/companies.csv)")
    args = p.parse_args(argv)

    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s", datefmt="%H:%M:%S")
    logging.getLogger("httpx").setLevel(logging.WARNING)  # one line per company is enough
    settings = config.load()
    if not settings.database_url:
        log.error("CRAWLER_DATABASE_URL or DATABASE_URL must be set")
        return 2

    conn = await store.connect(settings.database_url)
    async with conn:
        run = None
        if args.resume:
            run = await store.unfinished_run(conn)
            if run is None:
                log.error("no unfinished run to resume")
                return 1
            todo = await store.sources(conn, skip_attempted_since=run.started_at)
            log.info("resuming run %d from %s: %d companies left", run.id, run.started_at.astimezone(IST), len(todo))
        else:
            n = await seeds.load(conn, args.seeds)
            log.info("loaded %d companies from %s", n, args.seeds)
            if args.only:
                todo = await store.sources(conn, only=args.only)
                if not todo:
                    log.error("no company named %r in the seed file", args.only)
                    return 1
            else:
                stale = await store.unfinished_run(conn)
                if stale:
                    log.warning("run %d never finished; starting a new one (use --resume to continue it)", stale.id)
                run = await store.start_run(conn)
                todo = await store.sources(conn)

        deadline = stop_time(run.started_at, settings.stop_at_ist) if run else None
        async with PoliteClient(settings.user_agent, settings.robots_token,
                                settings.min_gap_seconds, settings.max_gap_seconds) as client:
            tally = await crawl(conn, client, settings, run, todo, deadline)

        if tally.not_started:
            log.warning("run budget reached at %s IST: %d companies not started; run is left open for --resume",
                        settings.stop_at_ist, tally.not_started)
        elif run:
            await store.finish_run(conn, run)
        if run:
            n_failed = await store.write_failed_csv(conn, run.started_at, FAILED_CSV)
            log.info("failed.csv: %d rows -> %s", n_failed, FAILED_CSV)

    log.info("SUMMARY companies ok %d, failed %d | jobs new %d, reopened %d, closed %d",
             tally.ok, tally.failed, tally.new, tally.reopened, tally.closed)
    for line in tally.failures:
        log.info("  failed: %s", line)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
