"""Ashby hosted job boards.

GET https://jobs.ashbyhq.com/{slug}            the board page
GET https://jobs.ashbyhq.com/{slug}/{job id}   one job page
Both pages carry their data as JSON in `window.__appData` (the board's job
list, or one posting with its description). robots.txt (checked 2026-11-12)
allows these pages and disallows /api/, and Ashby's separate posting API host
answers 401 for robots.txt, so only the public pages are read: one request for
the list, then one per India job that is new (at most MAX_NEW_DETAILS a night).
"""

import json
import logging
import re
from datetime import datetime, timezone

from adapters.base import DetailReads, ParseError, RawJob
from clean import cities_from, clean_html, in_india, work_mode_from
from fetcher import FetchError

log = logging.getLogger(__name__)
NAME = "ashby"
MAX_NEW_DETAILS = 300
_APP_DATA = re.compile(r"window\.__appData\s*=\s*")
_LD_JSON = re.compile(r'<script[^>]+type="application/ld\+json"[^>]*>(.*?)</script>', re.S | re.I)
_MODES = {"remote": "remote", "hybrid": "hybrid", "onsite": "onsite"}


def board_page(slug: str) -> str:
    return f"https://jobs.ashbyhq.com/{slug}"


def job_page(slug: str, job_id: str) -> str:
    return f"https://jobs.ashbyhq.com/{slug}/{job_id}"


def app_data(page: str) -> dict:
    m = _APP_DATA.search(page)
    if not m:
        raise ParseError("ashby: no __appData on the page")
    try:
        data, _ = json.JSONDecoder().raw_decode(page, m.end())
    except ValueError as e:
        raise ParseError(f"ashby: __appData is not JSON: {e}") from e
    if not isinstance(data, dict):
        raise ParseError("ashby: __appData is not an object")
    return data


def parse_list(page: str, slug: str) -> list[RawJob]:
    """Every listed job, without descriptions (partial)."""
    board = app_data(page).get("jobBoard")
    if not isinstance(board, dict) or not isinstance(board.get("jobPostings"), list):
        raise ParseError("ashby: expected jobBoard.jobPostings")
    jobs = []
    for p in board["jobPostings"]:
        job_id, title = p.get("id"), (p.get("title") or "").strip()
        if not job_id or not title:
            continue
        locations = [p.get("locationName") or ""] + [s.get("locationName") or "" for s in p.get("secondaryLocations") or []]
        jobs.append(_job(slug, str(job_id), title, locations, p.get("workplaceType"), partial=True))
    return jobs


def parse_detail(page: str, slug: str) -> RawJob:
    p = app_data(page).get("posting")
    if not isinstance(p, dict) or not p.get("id") or not (p.get("title") or "").strip():
        raise ParseError("ashby: expected a posting with an id and a title")
    locations = [p.get("locationName") or ""] + list(p.get("secondaryLocationNames") or [])
    job = _job(slug, str(p["id"]), p["title"].strip(), locations, p.get("workplaceType"), partial=False)
    job.description = clean_html(p.get("descriptionHtml"))
    job.posted_at = _posted(page)
    return job


def _job(slug: str, job_id: str, title: str, locations: list[str], workplace: str | None, partial: bool) -> RawJob:
    return RawJob(
        source_job_id=job_id,
        title=title,
        apply_url=job_page(slug, job_id),
        cities=cities_from(locations),
        in_india=in_india(locations),
        work_mode=_MODES.get((workplace or "").lower()) or work_mode_from(locations),
        partial=partial,
    )


def _posted(page: str) -> datetime | None:
    """datePosted from the page's JobPosting JSON-LD (the app data has no date)."""
    for raw in _LD_JSON.findall(page):
        try:
            value = json.loads(raw).get("datePosted")
            return datetime.fromisoformat(value).replace(tzinfo=timezone.utc) if value else None
        except (ValueError, AttributeError, TypeError):
            continue
    return None


async def fetch(client, slug: str, known) -> list[RawJob]:
    res = await client.get(board_page(slug))
    listed = parse_list(res.content.decode("utf-8", "replace"), slug)
    jobs, fetched, deferred = [], 0, 0
    reads = DetailReads(f"ashby {slug}")
    for job in listed:
        if job.in_india is False:
            continue  # clearly outside India: its page is never fetched
        if job.source_job_id in known:
            jobs.append(job)
            continue
        if fetched >= MAX_NEW_DETAILS:
            deferred += 1
            continue
        fetched += 1
        try:
            page = await client.get(job.apply_url)
        except FetchError as e:
            if e.status in (404, 410):  # closed between list and detail
                continue
            raise
        try:
            jobs.append(parse_detail(page.content.decode("utf-8", "replace"), slug))
            reads.ok()
        except ParseError as e:
            reads.skip(job.source_job_id, e)
    if deferred:
        log.info("ashby %s: %d new jobs left for the next night (cap %d)", slug, deferred, MAX_NEW_DETAILS)
    return jobs
