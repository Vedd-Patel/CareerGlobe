"""Nightly crawl of every careers source.

    python crawl.py --seeds ../seeds/companies.csv   # full run, start to finish
    python crawl.py --only "Groww"                   # one company, for testing
    python crawl.py --resume                         # continue a run that stopped
    python crawl.py --refresh                        # ignore ETags: re-parse every board
    python crawl.py --if-triggered                   # run only if the watchdog asked (exit 3: nothing to do)
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
import geocode
import seeds
import store
from heartbeat import Heartbeat
from adapters import ADAPTERS, ParseError
from fetcher import FetchError, PoliteClient, RobotsDisallowed
from jobdiff import diff, diff_not_modified
from urls import job_key

log = logging.getLogger("crawl")
IST = ZoneInfo("Asia/Kolkata")
DEFAULT_SEEDS = config.REPO_ROOT / "seeds" / "companies.csv"
FAILED_CSV = config.REPO_ROOT / "failed.csv"
MAX_FAILED_RATIO = 0.10  # spec: a night with more failures than this is a bad night
NOTHING_TO_DO = 3


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
        reason = f"no adapter for ATS {src.ats_type!r} yet"
        return await _fail(conn, db_lock, run, src, reason, tally, t0)

    etag = last_modified = None
    try:
        if adapter.fetch:
            async with db_lock:
                known = await store.known_jobs(conn, src.company_id)
            if adapter.name == "html":
                jobs = await adapter.fetch(client, src.ats_slug, known, selectors=src.selectors)
            else:
                jobs = await adapter.fetch(client, src.ats_slug, known)
            not_modified = False
        else:
            res = await client.get(adapter.board_url(src.ats_slug), etag=src.etag, last_modified=src.last_modified)
            etag, last_modified, not_modified = res.etag, res.last_modified, res.not_modified
            jobs = None if not_modified else adapter.parse(res.json(), src.ats_slug)
    except RobotsDisallowed as e:
        return await _fail(conn, db_lock, run, src, str(e), tally, t0, blocked=True)
    except FetchError as e:
        return await _fail(conn, db_lock, run, src, str(e), tally, t0)
    except (ParseError, ValueError) as e:
        return await _fail(conn, db_lock, run, src, f"unexpected response: {e}", tally, t0)

    if not_modified:
        compute, count = diff_not_modified, "unchanged (304)"
    else:
        fetched = [(job_key(src.company_id, src.ats_type, j.source_job_id, j.apply_url), j) for j in jobs]
        compute, count = (lambda existing: diff(existing, fetched)), f"{len(jobs)} jobs"

    try:
        async with db_lock:
            d = await store.apply_success(conn, run, src, compute, etag, last_modified)
    except ValueError as e:  # rolled back; e.g. a partial job that is not in the DB
        return await _fail(conn, db_lock, run, src, f"not saved: {e}", tally, t0)
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
    mode.add_argument("--if-triggered", action="store_true",
                      help="run a full crawl only if the watchdog requested one (exit 3 when there is nothing to do)")
    p.add_argument("--refresh", action="store_true",
                   help="ignore saved ETags and re-read every board (after changing an adapter or clean.py)")
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
        if args.if_triggered and not await store.take_trigger(conn):
            return NOTHING_TO_DO
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
            pinned, _ = await geocode.pin_locations(conn)
            log.info("loaded %d companies from %s (%d new map pins)", n, args.seeds, pinned)
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
        # Full and resumed runs report to the heartbeat monitor; --only runs do not.
        beat = Heartbeat("HEALTHCHECK_CRAWL_URL", settings.user_agent) if run else None
        if beat:
            await beat.start()
        try:
            async with PoliteClient(settings.user_agent, settings.robots_token,
                                    settings.min_gap_seconds, settings.max_gap_seconds) as client:
                if args.refresh:
                    for src in todo:
                        src.etag = src.last_modified = None
                tally = await crawl(conn, client, settings, run, todo, deadline)
        except BaseException as e:
            if beat:
                await beat.fail(f"crawl crashed: {type(e).__name__}")
            raise

        if tally.not_started:
            log.warning("run budget reached at %s IST: %d companies not started; run is left open for --resume",
                        settings.stop_at_ist, tally.not_started)
        elif run:
            await store.finish_run(conn, run)
        if run:
            n_failed = await store.write_failed_csv(conn, run.started_at, FAILED_CSV)
            log.info("failed.csv: %d rows -> %s", n_failed, FAILED_CSV)

    summary = summarize(tally)
    log.info("%s", summary.splitlines()[0])
    for line in tally.failures:
        log.info("  failed: %s", line)
    if beat:
        await (beat.fail(summary) if bad_night(tally) else beat.success(summary))
    return 0


def summarize(tally: Tally) -> str:
    lines = [f"SUMMARY companies ok {tally.ok}, failed {tally.failed}, not started {tally.not_started} | "
             f"jobs new {tally.new}, reopened {tally.reopened}, closed {tally.closed}"]
    return "\n".join(lines + [f"failed: {f}" for f in tally.failures])


def bad_night(tally: Tally) -> bool:
    """More than 10% of attempted companies failed, or the time budget cut the run short."""
    attempted = tally.ok + tally.failed
    return tally.not_started > 0 or (attempted > 0 and tally.failed / attempted > MAX_FAILED_RATIO)


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
