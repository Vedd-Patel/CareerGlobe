"""End to end against the local Postgres (docker compose up -d db).

Creates a throwaway database `careerglobe_test`, applies db/migrations, then
runs real crawls night after night against fake ATS servers. Skipped when
DATABASE_URL is unset or the database is not reachable.
"""

import hashlib
import json
import os
from pathlib import Path

import httpx
import psycopg
import pytest
from psycopg.conninfo import make_conninfo

import config
import crawl
import seeds
import store
from fetcher import PoliteClient

pytestmark = pytest.mark.anyio
MIGRATIONS = config.REPO_ROOT / "db" / "migrations"
TEST_DB = "careerglobe_test"


def _base_url():
    from dotenv import load_dotenv

    load_dotenv(config.REPO_ROOT / ".env", override=False)
    return os.environ.get("DATABASE_URL")


def _up_sql(path: Path) -> str:
    text = path.read_text()
    return text.split("-- +goose Up", 1)[1].split("-- +goose Down", 1)[0]


@pytest.fixture
async def db():
    base = _base_url()
    if not base:
        pytest.skip("DATABASE_URL not set")
    try:
        admin = await psycopg.AsyncConnection.connect(base, autocommit=True, connect_timeout=3)
    except psycopg.OperationalError:
        pytest.skip("local Postgres not reachable (docker compose up -d db)")
    async with admin:
        await admin.execute(f"DROP DATABASE IF EXISTS {TEST_DB} WITH (FORCE)")
        await admin.execute(f"CREATE DATABASE {TEST_DB}")
    url = make_conninfo(base, dbname=TEST_DB)
    conn = await store.connect(url)
    for f in sorted(MIGRATIONS.glob("*.sql")):
        await conn.execute(_up_sql(f))
    yield conn
    await conn.close()


class FakeATS:
    """Greenhouse board for "acme" (with ETags) and a Lever board for "down" that is broken."""

    def __init__(self):
        self.jobs = [f"{i}" for i in range(10)]
        self.lever_status = 503

    def body(self):
        return json.dumps({"jobs": [
            {"id": int(i), "title": f"Engineer {i}", "absolute_url": f"https://job-boards.greenhouse.io/acme/jobs/{i}",
             "location": {"name": "Bengaluru, India"}, "first_published": "2026-10-01T10:00:00+05:30",
             "content": "&lt;p&gt;Build things&lt;/p&gt;"}
            for i in self.jobs]}).encode()

    def __call__(self, request: httpx.Request) -> httpx.Response:
        if request.url.path == "/robots.txt":
            return httpx.Response(404)
        if request.url.host == "api.lever.co":
            return httpx.Response(self.lever_status, json=[])
        body = self.body()
        etag = '"' + hashlib.md5(body).hexdigest() + '"'
        if request.headers.get("If-None-Match") == etag:
            return httpx.Response(304, headers={"ETag": etag})
        return httpx.Response(200, content=body, headers={"ETag": etag, "Content-Type": "application/json"})


SEEDS = """name,website,careers_url,city,state,type,ats,note
Acme,https://acme.example,https://boards.greenhouse.io/acme,Bengaluru,Karnataka,product,,
Down Corp,https://down.example,https://jobs.lever.co/down,Pune,Maharashtra,service,,
"""


@pytest.fixture
def env(tmp_path):
    path = tmp_path / "companies.csv"
    path.write_text(SEEDS)
    settings = config.Settings(database_url=None, user_agent="CareerGlobeBot/test", min_gap_seconds=0,
                               max_gap_seconds=0, max_hosts_in_flight=8, stop_at_ist="05:30")
    return path, settings, FakeATS()


async def night(conn, settings, ats, only=None):
    """One full run (or an --only run when `only` is given)."""
    run = None if only else await store.start_run(conn)
    sources = await store.sources(conn, only=only)
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(ats)) as client:
        tally = await crawl.crawl(conn, client, settings, run, sources, deadline=None)
    if run:
        await store.finish_run(conn, run)
    return tally


async def jobs(conn):
    cur = await conn.execute("SELECT source_job_id, status, missed_runs, closed_at IS NOT NULL FROM jobs ORDER BY id")
    return {r[0]: r[1:] for r in await cur.fetchall()}


async def source(conn, company):
    cur = await conn.execute(
        """SELECT s.status, s.fail_count, s.etag IS NOT NULL, s.last_error
           FROM careers_sources s JOIN companies c ON c.id = s.company_id WHERE c.name = %s""", (company,))
    return await cur.fetchone()


async def test_seeds_load_companies_locations_and_detect_ats(db, env):
    path, _, _ = env
    assert await seeds.load(db, path) == 2
    assert await seeds.load(db, path) == 2  # idempotent
    cur = await db.execute(
        """SELECT c.name, c.slug, c.type, s.ats_type, s.ats_slug, l.city, l.is_hq
           FROM companies c JOIN careers_sources s ON s.company_id = c.id
           JOIN company_locations l ON l.company_id = c.id ORDER BY c.name""")
    assert await cur.fetchall() == [
        ("Acme", "acme", "product", "greenhouse", "acme", "Bengaluru", True),
        ("Down Corp", "down-corp", "service", "lever", "down", "Pune", True),
    ]


async def test_nights_end_to_end(db, env, tmp_path):
    path, settings, ats = env
    await seeds.load(db, path)

    # Night 1: Acme's 10 jobs arrive; Down Corp's board is erroring.
    t = await night(db, settings, ats)
    assert (t.ok, t.failed, t.new) == (1, 1, 10)
    assert all(v == ("open", 0, False) for v in (await jobs(db)).values())
    assert (await source(db, "Acme"))[:3] == ("active", 0, True)
    assert (await source(db, "Down Corp"))[:2] == ("active", 1)

    # Night 2: nothing changed -> 304, nothing closes.
    t = await night(db, settings, ats)
    assert (t.ok, t.new, t.closed) == (1, 0, 0)
    assert all(v[0] == "open" for v in (await jobs(db)).values())

    # Night 3: job 3 removed -> first miss, still open.
    ats.jobs.remove("3")
    t = await night(db, settings, ats)
    assert (await jobs(db))["3"] == ("open", 1, False)

    # Night 4: unchanged board (304) -> second successful crawl without job 3 -> closed.
    t = await night(db, settings, ats)
    assert t.closed == 1
    assert (await jobs(db))["3"] == ("closed", 2, True)
    # Down Corp has now failed 4 nights in a row -> flagged for review.
    assert (await source(db, "Down Corp"))[:2] == ("review", 4)

    # Night 5: Acme's page breaks and returns 0 jobs -> flagged, nothing closes.
    saved = list(ats.jobs)
    ats.jobs = []
    t = await night(db, settings, ats)
    status, _, _, error = await source(db, "Acme")
    assert status == "broken" and "0 jobs returned; 9 were open" in error
    assert sum(1 for v in (await jobs(db)).values() if v[0] == "open") == 9

    # Night 6: page fixed, job 3 is back -> reopened; Down Corp recovers too.
    ats.jobs = saved + ["3"]
    ats.lever_status = 200
    t = await night(db, settings, ats)
    assert (t.ok, t.failed, t.reopened) == (2, 0, 1)
    assert (await jobs(db))["3"] == ("open", 0, False)
    assert (await source(db, "Acme"))[0] == "active"
    assert (await source(db, "Down Corp"))[:2] == ("active", 0)

    # crawl_runs: one row per night with the right counters.
    cur = await db.execute(
        "SELECT companies_ok, companies_failed, jobs_new, jobs_closed, finished_at IS NOT NULL FROM crawl_runs ORDER BY id")
    assert await cur.fetchall() == [
        (1, 1, 10, 0, True), (1, 1, 0, 0, True), (1, 1, 0, 0, True),
        (1, 1, 0, 1, True), (0, 2, 0, 0, True), (2, 0, 0, 0, True),
    ]


async def test_first_seen_survives_reopen(db, env):
    path, settings, ats = env
    await seeds.load(db, path)
    await night(db, settings, ats)
    cur = await db.execute("SELECT first_seen FROM jobs WHERE source_job_id = '3'")
    first = (await cur.fetchone())[0]
    ats.jobs.remove("3")
    await night(db, settings, ats)
    await night(db, settings, ats)
    ats.jobs.append("3")
    await night(db, settings, ats)
    cur = await db.execute("SELECT first_seen, status FROM jobs WHERE source_job_id = '3'")
    assert await cur.fetchone() == (first, "open")


async def test_only_run_is_not_recorded_in_crawl_runs(db, env):
    path, settings, ats = env
    await seeds.load(db, path)
    t = await night(db, settings, ats, only="acme")
    assert (t.ok, t.new) == (1, 10)
    cur = await db.execute("SELECT count(*) FROM crawl_runs")
    assert (await cur.fetchone())[0] == 0


async def test_resume_skips_companies_already_attempted(db, env, tmp_path):
    path, settings, ats = env
    await seeds.load(db, path)
    run = await store.start_run(db)
    # The run "crashes" after crawling only Acme.
    acme = await store.sources(db, only="Acme")
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(ats)) as client:
        await crawl.crawl(db, client, settings, run, acme, deadline=None)

    resumed = await store.unfinished_run(db)
    assert resumed.id == run.id
    left = await store.sources(db, skip_attempted_since=resumed.started_at)
    assert [s.company for s in left] == ["Down Corp"]

    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(ats)) as client:
        await crawl.crawl(db, client, settings, resumed, left, deadline=None)
    await store.finish_run(db, resumed)
    assert await store.unfinished_run(db) is None

    out = tmp_path / "failed.csv"
    assert await store.write_failed_csv(db, run.started_at, out) == 1
    text = out.read_text()
    assert "Down Corp" in text and "HTTP 503" in text and "Acme" not in text


async def test_robots_block_marks_source_blocked(db, env):
    path, settings, ats = env
    await seeds.load(db, path)

    def blocking(request):
        if request.url.path == "/robots.txt":
            return httpx.Response(200, text="User-agent: *\nDisallow: /\n")
        return ats(request)

    await night(db, settings, blocking, only="Acme")
    status, fails, _, error = await source(db, "Acme")
    assert status == "blocked" and "robots.txt disallows" in error


async def test_company_without_adapter_fails_cleanly(db, env, tmp_path):
    _, settings, ats = env
    path = tmp_path / "custom.csv"
    path.write_text("name,website,careers_url,city,state,type,ats\nZoho,https://www.zoho.com,https://careers.zohocorp.com,Chennai,Tamil Nadu,product,\n")
    await seeds.load(db, path)
    t = await night(db, settings, ats, only="Zoho")
    assert t.failed == 1 and "no adapter" in t.failures[0]


def test_stop_time_is_next_0530_ist():
    from datetime import datetime

    start = datetime(2026, 10, 20, 1, 30, tzinfo=crawl.IST)
    assert crawl.stop_time(start, "05:30") == datetime(2026, 10, 20, 5, 30, tzinfo=crawl.IST)
    later = datetime(2026, 10, 20, 14, 0, tzinfo=crawl.IST)
    assert crawl.stop_time(later, "05:30") == datetime(2026, 10, 21, 5, 30, tzinfo=crawl.IST)
