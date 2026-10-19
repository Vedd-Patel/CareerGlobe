"""Postgres reads and writes for one crawl."""

import csv
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import psycopg
from psycopg.rows import dict_row

from adapters.base import RawJob
from jobdiff import CLOSE_AFTER_MISSES, Diff, ExistingJob

REVIEW_AFTER_FAILURES = 3


@dataclass
class Source:
    id: int
    company_id: int
    company: str
    careers_url: str
    ats_type: str | None
    ats_slug: str | None
    etag: str | None
    last_modified: str | None


@dataclass
class Run:
    id: int
    started_at: datetime


async def start_run(conn) -> Run:
    cur = await conn.execute("INSERT INTO crawl_runs DEFAULT VALUES RETURNING id, started_at")
    row = await cur.fetchone()
    return Run(row[0], row[1])


async def unfinished_run(conn) -> Run | None:
    cur = await conn.execute(
        "SELECT id, started_at FROM crawl_runs WHERE finished_at IS NULL ORDER BY started_at DESC LIMIT 1"
    )
    row = await cur.fetchone()
    return Run(row[0], row[1]) if row else None


async def finish_run(conn, run: Run) -> dict:
    cur = await conn.execute(
        """UPDATE crawl_runs SET finished_at = now() WHERE id = %s
           RETURNING companies_ok, companies_failed, jobs_new, jobs_closed""",
        (run.id,),
    )
    ok, failed, new, closed = await cur.fetchone()
    return {"companies_ok": ok, "companies_failed": failed, "jobs_new": new, "jobs_closed": closed}


async def sources(conn, only: str | None = None, skip_attempted_since: datetime | None = None) -> list[Source]:
    """Sources to crawl, least recently successful first, so companies left
    over from a run that hit its time budget go first the next night."""
    where, params = ["s.status <> 'disabled'"], []
    if only:
        where.append("lower(c.name) = lower(%s)")
        params.append(only)
    if skip_attempted_since:
        where.append("(s.last_attempt_at IS NULL OR s.last_attempt_at < %s)")
        params.append(skip_attempted_since)
    cur = await conn.execute(
        f"""SELECT s.id, s.company_id, c.name, s.careers_url, s.ats_type, s.ats_slug, s.etag, s.last_modified
            FROM careers_sources s JOIN companies c ON c.id = s.company_id
            WHERE {' AND '.join(where)}
            ORDER BY s.last_ok_at ASC NULLS FIRST, c.name""",
        params,
    )
    return [Source(*row) for row in await cur.fetchall()]


async def apply_success(conn, run: Run | None, src: Source, compute, etag, last_modified) -> Diff:
    """One transaction: lock the company, load its jobs, diff, write, record.
    `compute(existing) -> Diff` runs inside the lock, so two writers can never
    diff against stale rows."""
    async with conn.transaction():
        await conn.execute("SELECT 1 FROM companies WHERE id = %s FOR UPDATE", (src.company_id,))
        cur = await conn.execute(
            "SELECT id, job_key, status, missed_runs FROM jobs WHERE company_id = %s", (src.company_id,)
        )
        d: Diff = compute([ExistingJob(*r) for r in await cur.fetchall()])

        if d.broken:
            await _mark(conn, src, "broken", d.broken_reason, failed=False)
            await _count(conn, run, ok=False)
            return d

        await _write_jobs(conn, src.company_id, d)
        await conn.execute(
            """UPDATE careers_sources SET status = 'active', fail_count = 0, last_error = NULL,
                   last_attempt_at = now(), last_ok_at = now(), etag = %s, last_modified = %s
               WHERE id = %s""",
            (etag, last_modified, src.id),
        )
        await _count(conn, run, ok=True, new=len(d.inserts), closed=len(d.closes))
        return d


async def record_failure(conn, run: Run | None, src: Source, error: str, blocked: bool = False) -> None:
    async with conn.transaction():
        await _mark(conn, src, "blocked" if blocked else None, error, failed=True)
        await _count(conn, run, ok=False)


async def _mark(conn, src: Source, status: str | None, error: str, failed: bool) -> None:
    # 3 failures in a row -> 'review' (flag for a human); still crawled nightly.
    await conn.execute(
        """UPDATE careers_sources
           SET fail_count = fail_count + %(inc)s,
               status = COALESCE(%(status)s,
                                 CASE WHEN fail_count + %(inc)s >= %(review)s THEN 'review' ELSE status END),
               last_error = %(error)s, last_attempt_at = now()
           WHERE id = %(id)s""",
        {"inc": 1 if failed else 0, "status": status, "review": REVIEW_AFTER_FAILURES,
         "error": error[:1000], "id": src.id},
    )


async def _count(conn, run: Run | None, ok: bool, new: int = 0, closed: int = 0) -> None:
    if run is None:
        return
    await conn.execute(
        """UPDATE crawl_runs SET companies_ok = companies_ok + %s, companies_failed = companies_failed + %s,
               jobs_new = jobs_new + %s, jobs_closed = jobs_closed + %s
           WHERE id = %s""",
        (int(ok), int(not ok), new, closed, run.id),
    )


def _fields(j: RawJob) -> tuple:
    return (j.title, j.description, j.cities, j.work_mode, j.salary_min, j.salary_max, j.apply_url, j.posted_at)


async def _write_jobs(conn, company_id: int, d: Diff) -> None:
    async with conn.cursor() as cur:
        if d.inserts:
            await cur.executemany(
                """INSERT INTO jobs (company_id, job_key, source_job_id, title, description, cities, work_mode,
                                     salary_min, salary_max, apply_url, posted_at)
                   VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)""",
                [(company_id, key, j.source_job_id, *_fields(j)) for key, j in d.inserts],
            )
        for rows, reopen in ((d.updates, False), (d.reopens, True)):
            if rows:
                await cur.executemany(
                    f"""UPDATE jobs SET title = %s, description = %s, cities = %s, work_mode = %s,
                            salary_min = %s, salary_max = %s, apply_url = %s,
                            posted_at = COALESCE(%s, posted_at), last_seen = now(), missed_runs = 0
                            {", status = 'open', closed_at = NULL" if reopen else ""}
                        WHERE id = %s""",
                    [(*_fields(j), job_id) for job_id, j in rows],
                )
        if d.touch:
            await cur.execute("UPDATE jobs SET last_seen = now() WHERE id = ANY(%s)", (d.touch,))
        if d.misses:
            await cur.executemany("UPDATE jobs SET missed_runs = %s WHERE id = %s",
                                  [(missed, job_id) for job_id, missed in d.misses])
        if d.closes:
            await cur.execute(
                "UPDATE jobs SET status = 'closed', closed_at = now(), missed_runs = %s WHERE id = ANY(%s)",
                (CLOSE_AFTER_MISSES, d.closes),
            )


async def write_failed_csv(conn, run_started: datetime, path: Path) -> int:
    """Every source attempted in this run that did not end in success."""
    async with conn.cursor(row_factory=dict_row) as cur:
        await cur.execute(
            """SELECT c.name AS company, s.careers_url, s.ats_type AS ats, s.status, s.fail_count,
                      s.last_error AS reason, s.last_attempt_at
               FROM careers_sources s JOIN companies c ON c.id = s.company_id
               WHERE s.last_attempt_at >= %s AND (s.last_ok_at IS NULL OR s.last_ok_at < s.last_attempt_at)
               ORDER BY c.name""",
            (run_started,),
        )
        rows = await cur.fetchall()
    with path.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=["company", "careers_url", "ats", "status", "fail_count", "reason",
                                          "last_attempt_at"])
        w.writeheader()
        w.writerows(rows)
    return len(rows)


async def connect(url: str) -> psycopg.AsyncConnection:
    return await psycopg.AsyncConnection.connect(url, autocommit=True)
