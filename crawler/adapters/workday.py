"""Workday career sites (myworkdayjobs.com).

List:   POST https://{tenant}.{wdN}.myworkdayjobs.com/wday/cxs/{tenant}/{site}/jobs
        {"appliedFacets": {...}, "limit": 20, "offset": n, "searchText": ""}
Detail: GET  https://{tenant}.{wdN}.myworkdayjobs.com/wday/cxs/{tenant}/{site}{externalPath}

Undocumented: these are the calls Workday's own careers page makes. robots.txt
on the tenants checked (2026-10-21) disallows only /refreshFacet/ and some
internal sites, never /wday/cxs/. Seed slug: "{tenant}.{wdN}/{site}".

- Only jobs in India: the first page's facets are searched for an "India"
  country value (the same ID on every tenant), else for every `locations`
  value naming India. No India filter at all fails the company instead of
  crawling the whole world (or closing anything).
- The list has no descriptions; one detail request per job is made only for
  jobs not yet in the DB, at most MAX_NEW_DETAILS per night. The rest wait for
  the next night. Jobs already stored come back `partial`.
- Workday stops counting at 2000; a list that size may be cut off, so it fails
  rather than close jobs that were simply not listed.
"""

import logging
import re
from datetime import datetime, timezone

from adapters.base import ParseError, RawJob
from clean import cities_from, clean_html, work_mode_from
from fetcher import FetchError

log = logging.getLogger(__name__)
NAME = "workday"
PAGE_SIZE = 20  # Workday rejects larger pages
LIST_CAP = 2000  # Workday's `total` never goes above this
MAX_NEW_DETAILS = 300

_SLUG = re.compile(r"^([a-z0-9-]+)\.(wd\d+)/([A-Za-z0-9_-]+)$")
_INDIA = re.compile(r"\bindia\b", re.IGNORECASE)


def parse_slug(slug: str) -> tuple[str, str, str]:
    m = _SLUG.match(slug)
    if not m:
        raise ParseError(f"workday: bad slug {slug!r} (want tenant.wdN/site)")
    return m.group(1), m.group(2), m.group(3)


def api_base(slug: str) -> str:
    tenant, wd, site = parse_slug(slug)
    return f"https://{tenant}.{wd}.myworkdayjobs.com/wday/cxs/{tenant}/{site}"


def public_url(slug: str, external_path: str) -> str:
    tenant, wd, site = parse_slug(slug)
    return f"https://{tenant}.{wd}.myworkdayjobs.com/{site}{external_path}"


def posting_id(external_path: str) -> str:
    """'/job/India-Pune/CAD-Engineer_JR2014480-1' -> 'JR2014480-1'. One requisition
    can have several postings (-1, -2), so the req ID alone is not unique."""
    tail = external_path.rstrip("/").rsplit("/", 1)[-1]
    return tail.rsplit("_", 1)[-1] if "_" in tail else tail


def india_facets(facets) -> dict[str, list[str]]:
    """appliedFacets that limit the list to India, or {} when the site has none."""
    leaves: list[tuple[str, dict]] = []

    def walk(groups):
        for f in groups or []:
            for v in f.get("values") or []:
                if "values" in v:
                    walk([v])
                elif v.get("id"):
                    leaves.append((f.get("facetParameter") or "", v))

    walk(facets)
    for param, v in leaves:
        if (v.get("descriptor") or "").strip().lower() == "india":
            return {param: [v["id"]]}
    ids = [v["id"] for param, v in leaves if param == "locations" and _INDIA.search(v.get("descriptor") or "")]
    return {"locations": ids} if ids else {}


def list_job(posting: dict, slug: str) -> RawJob | None:
    """A known job seen in the list: enough to keep it open."""
    path, title = posting.get("externalPath"), (posting.get("title") or "").strip()
    if not path or not title:
        return None
    return RawJob(source_job_id=posting_id(path), title=title, apply_url=public_url(slug, path), partial=True)


def parse_detail(data, slug: str, external_path: str) -> RawJob:
    info = data.get("jobPostingInfo") if isinstance(data, dict) else None
    if not isinstance(info, dict) or not info.get("title"):
        raise ParseError("workday: expected jobPostingInfo with a title")
    locations = [info.get("location") or ""] + list(info.get("additionalLocations") or [])
    return RawJob(
        source_job_id=posting_id(external_path),
        title=info["title"].strip(),
        apply_url=public_url(slug, external_path),
        description=clean_html(info.get("jobDescription")),
        cities=cities_from(locations),
        work_mode=work_mode_from([info.get("remoteType") or ""] + locations),
        posted_at=_date(info.get("startDate")),
    )


async def fetch(client, slug: str, known: set[str]) -> list[RawJob]:
    base = api_base(slug)
    body = {"appliedFacets": {}, "limit": PAGE_SIZE, "offset": 0, "searchText": ""}
    first = _page(await client.post_json(f"{base}/jobs", body))
    if not first.get("total") and not first["jobPostings"]:
        return []  # no open jobs anywhere (the sanity check guards a sudden drop)
    facets = india_facets(first.get("facets"))
    if not facets:
        raise ParseError("workday: no India location filter on this site")
    body["appliedFacets"] = facets

    postings, total, offset = [], None, 0
    while total is None or offset < total:
        body["offset"] = offset
        page = _page(await client.post_json(f"{base}/jobs", body))
        if total is None:
            total = int(page.get("total") or 0)  # later pages may report 0
            if total >= LIST_CAP:
                raise ParseError(f"workday: {total} India jobs hits Workday's {LIST_CAP} cap; list may be cut off")
        batch = page.get("jobPostings") or []
        postings += batch
        if len(batch) < PAGE_SIZE:
            break
        offset += PAGE_SIZE

    jobs, seen, fetched, deferred = [], set(), 0, 0
    for p in postings:
        path = p.get("externalPath")
        if not path or posting_id(path) in seen:
            continue  # the list can shift while paging and repeat a job
        seen.add(posting_id(path))
        if posting_id(path) in known:
            if job := list_job(p, slug):
                jobs.append(job)
            continue
        if fetched >= MAX_NEW_DETAILS:
            deferred += 1
            continue
        fetched += 1
        try:
            res = await client.get(base + path)
        except FetchError as e:
            if e.status in (404, 410):  # closed between list and detail
                continue
            raise
        try:
            jobs.append(parse_detail(res.json(), slug, path))
        except ValueError as e:
            raise ParseError(f"workday: bad detail JSON for {path}: {e}") from e
    if deferred:
        log.info("workday %s: %d new jobs left for the next night (cap %d)", slug, deferred, MAX_NEW_DETAILS)
    return jobs


def _page(res) -> dict:
    try:
        data = res.json()
    except ValueError as e:
        raise ParseError(f"workday: list is not JSON: {e}") from e
    if not isinstance(data, dict) or not isinstance(data.get("jobPostings"), list):
        raise ParseError("workday: expected an object with a 'jobPostings' list")
    return data


def _date(value: str | None) -> datetime | None:
    try:
        return datetime.fromisoformat(value).replace(tzinfo=timezone.utc) if value else None
    except ValueError:
        return None
