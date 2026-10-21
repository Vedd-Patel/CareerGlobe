"""Eightfold career sites (PCSX API), e.g. Microsoft.

List:   GET https://{host}/api/pcsx/search?domain={domain}&query=&location=India&start={n}
        10 positions a page (fixed).
Detail: GET https://{host}/api/pcsx/position_details?position_id={id}&domain={domain}&hl=en

Undocumented: the calls the careers site makes. Microsoft's robots.txt
(apply.careers.microsoft.com, checked 2026-10-22) is "Disallow: /" with
"Allow: /api/pcsx" and "Allow: /careers". The host answered 429 at a 2 s pace,
so this adapter asks the client for MIN_GAP seconds between requests.
Seed slug: "{host}|{domain}".

- Only India locations are kept (standardizedLocations ending in "IN").
- Descriptions only for positions not yet in the DB, at most MAX_NEW_DETAILS
  a night.
"""

import logging
import re
from datetime import datetime, timezone
from urllib.parse import quote

from adapters.base import ParseError, RawJob
from clean import cities_from, clean_html
from fetcher import FetchError

log = logging.getLogger(__name__)
NAME = "eightfold"
PAGE_SIZE = 10
MAX_PAGES = 300
MAX_NEW_DETAILS = 300
MIN_GAP = 6.0
_SLUG = re.compile(r"^([a-z0-9.-]+\.[a-z]{2,})\|([a-z0-9.-]+\.[a-z]{2,})$")
_MODES = {"remote": "remote", "hybrid": "hybrid", "onsite": "onsite"}


def parse_slug(slug: str) -> tuple[str, str]:
    m = _SLUG.match(slug or "")
    if not m:
        raise ParseError(f"eightfold: bad slug {slug!r} (want host|domain)")
    return m.group(1), m.group(2)


def search_url(slug: str, start: int) -> str:
    host, domain = parse_slug(slug)
    return f"https://{host}/api/pcsx/search?domain={quote(domain)}&query=&location=India&start={start}"


def detail_url(slug: str, position_id: str) -> str:
    host, domain = parse_slug(slug)
    return f"https://{host}/api/pcsx/position_details?position_id={position_id}&domain={quote(domain)}&hl=en"


def india_places(position: dict) -> list[str]:
    """India entries of standardizedLocations ("Hyderabad, TS, IN" or just "IN")."""
    return [s for s in position.get("standardizedLocations") or [] if re.search(r"(^|,\s*)IN$", s.strip())]


def parse_search(data, slug: str) -> tuple[list[RawJob], int]:
    """(India positions on this page as partial jobs, total count)."""
    body = data.get("data") if isinstance(data, dict) else None
    if not isinstance(body, dict) or not isinstance(body.get("positions"), list):
        raise ParseError("eightfold: expected data.positions")
    host, _ = parse_slug(slug)
    jobs = []
    for p in body["positions"]:
        title = (p.get("name") or "").strip()
        places = india_places(p)
        if not p.get("id") or not title or not places:
            continue
        url = p.get("positionUrl") or f"/careers/job/{p['id']}"
        jobs.append(RawJob(
            source_job_id=str(p["id"]),
            title=title,
            apply_url=url if url.startswith("http") else f"https://{host}{url}",
            cities=cities_from([s for s in places if s != "IN"]),
            work_mode=_MODES.get((p.get("workLocationOption") or "").lower()),
            posted_at=_ts(p.get("postedTs")),
            partial=True,
        ))
    return jobs, int(body.get("count") or 0)


def parse_detail(data, listed: RawJob) -> RawJob:
    body = data.get("data", data) if isinstance(data, dict) else None
    if not isinstance(body, dict) or "jobDescription" not in body:
        raise ParseError(f"eightfold: no jobDescription for position {listed.source_job_id}")
    return RawJob(
        source_job_id=listed.source_job_id,
        title=(body.get("name") or "").strip() or listed.title,
        apply_url=body.get("publicUrl") or listed.apply_url,
        description=clean_html(body.get("jobDescription")),
        cities=listed.cities,
        work_mode=listed.work_mode,
        posted_at=listed.posted_at,
    )


async def fetch(client, slug: str, known) -> list[RawJob]:
    host, _ = parse_slug(slug)
    client.min_gap(host, MIN_GAP)
    listed: dict[str, RawJob] = {}
    total = None
    for page in range(MAX_PAGES):
        res = await client.get(search_url(slug, page * PAGE_SIZE))
        try:
            data = res.json()
        except ValueError as e:
            raise ParseError(f"eightfold: not JSON: {e}") from e
        jobs, count = parse_search(data, slug)
        total = count if total is None else total
        for j in jobs:
            listed.setdefault(j.source_job_id, j)
        if len(data["data"]["positions"]) < PAGE_SIZE or (page + 1) * PAGE_SIZE >= total:
            break
    else:
        raise ParseError(f"eightfold: more than {MAX_PAGES} pages; list may be cut off")

    out, fetched, deferred = [], 0, 0
    for job_id, job in listed.items():
        if job_id in known:
            out.append(job)
            continue
        if fetched >= MAX_NEW_DETAILS:
            deferred += 1
            continue
        fetched += 1
        try:
            res = await client.get(detail_url(slug, job_id))
        except FetchError as e:
            if e.status in (404, 410):
                continue
            raise
        try:
            out.append(parse_detail(res.json(), job))
        except ValueError as e:
            raise ParseError(f"eightfold: bad detail JSON for {job_id}: {e}") from e
    if deferred:
        log.info("eightfold %s: %d new jobs left for the next night (cap %d)", slug, deferred, MAX_NEW_DETAILS)
    return out


def _ts(value) -> datetime | None:
    try:
        return datetime.fromtimestamp(int(value), tz=timezone.utc) if value else None
    except (TypeError, ValueError, OverflowError):
        return None
