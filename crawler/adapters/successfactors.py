"""SAP SuccessFactors career sites, classic template (careers.{company}.com).

List:   GET https://{host}/search/?q=&locationsearch=India&startrow={n}
        Server-rendered rows: title + link, location, date. 25 (or 50) a page.
Detail: GET https://{host}/job/{url-title}/{id}/
        schema.org microdata: itemprop title and description; .jobGeoLocation.

robots.txt on every site checked (2026-10-21) disallows /services/, apply and
talent-community pages, never /search/ or /job/. Seed slug: the host.

- locationsearch=India matches the job's country, also when the location is
  written "Pune, MH, IN" (checked: Tata Motors lists 144 with and without it).
- Detail pages only for jobs not yet in the DB, at most MAX_NEW_DETAILS a night;
  the list already has title, city and date, so known jobs come back complete
  except for the description (sent as partial so it is kept).
- Newer SuccessFactors sites (Wipro, HCLTech) render jobs with JavaScript from
  /services/, which robots.txt disallows; they have no table rows and fail here.
"""

import logging
import re
from datetime import datetime, timezone
from urllib.parse import urljoin

from selectolax.lexbor import LexborHTMLParser

from adapters.base import DetailReads, ParseError, RawJob
from clean import cities_from, clean_html, work_mode_from
from fetcher import FetchError

log = logging.getLogger(__name__)
NAME = "successfactors"
MAX_NEW_DETAILS = 300
MAX_PAGES = 100

_HOST = re.compile(r"^[a-z0-9.-]+\.[a-z]{2,}$")
_JOB_ID = re.compile(r"/job/[^/]+/(\d+)/?$")
_TOTAL = re.compile(r"of\s*([\d,]+)\s*$")


def search_url(host: str, startrow: int) -> str:
    if not _HOST.match(host):
        raise ParseError(f"successfactors: bad slug {host!r} (want the careers host)")
    return f"https://{host}/search/?q=&locationsearch=India&startrow={startrow}"


def parse_search(html: str, host: str) -> tuple[list[RawJob], int | None]:
    """(jobs on this page, total from "Results 1 – 25 of 144")."""
    tree = LexborHTMLParser(html)
    label = tree.css_first(".paginationLabel")
    total = None
    if label:
        m = _TOTAL.search(label.text(strip=True))
        total = int(m.group(1).replace(",", "")) if m else None
    rows = tree.css("tr.data-row")
    if not rows and total is None:
        if tree.css_first("table#searchresults") is None:
            raise ParseError("successfactors: no search results table (JavaScript-only site?)")
        return [], 0
    jobs = []
    for row in rows:
        link = row.css_first("a.jobTitle-link")
        href = link.attributes.get("href") if link else None
        m = _JOB_ID.search(href or "")
        title = link.text(strip=True) if link else ""
        if not m or not title:
            continue
        loc_node = row.css_first("td.colLocation .jobLocation") or row.css_first(".jobLocation")
        location = loc_node.text(strip=True) if loc_node else ""
        date_node = row.css_first(".jobDate")
        jobs.append(RawJob(
            source_job_id=m.group(1),
            title=title,
            apply_url=urljoin(f"https://{host}/", href),
            cities=cities_from([location]),
            work_mode=work_mode_from([location]),
            posted_at=_date(date_node.text(strip=True) if date_node else None),
            partial=True,
        ))
    return jobs, total


def parse_job(html: str, listed: RawJob) -> RawJob:
    """The listed job plus its description (and location, if the page has it)."""
    tree = LexborHTMLParser(html)
    desc = tree.css_first('[itemprop="description"]')
    if desc is None:
        raise ParseError(f"successfactors: no itemprop=description on {listed.apply_url}")
    title = tree.css_first('[itemprop="title"]')
    geo = tree.css_first(".jobGeoLocation")
    cities = cities_from([geo.text(strip=True)]) if geo else []
    return RawJob(
        source_job_id=listed.source_job_id,
        title=(title.text(strip=True) if title else "") or listed.title,
        apply_url=listed.apply_url,
        description=clean_html(desc.html),
        cities=cities or listed.cities,
        work_mode=listed.work_mode,
        posted_at=listed.posted_at,
    )


async def fetch(client, host: str, known) -> list[RawJob]:
    listed: dict[str, RawJob] = {}
    startrow, total = 0, None
    for _ in range(MAX_PAGES):
        res = await client.get(search_url(host, startrow))
        jobs, page_total = parse_search(res.content.decode("utf-8", "replace"), host)
        if total is None:
            total = page_total or 0
        for j in jobs:
            listed.setdefault(j.source_job_id, j)  # the list can shift while paging
        if not jobs:
            break
        startrow += len(jobs)
        if startrow >= total:
            break
    else:
        raise ParseError(f"successfactors: more than {MAX_PAGES} pages; list may be cut off")

    out, fetched, deferred = [], 0, 0
    reads = DetailReads(f"successfactors {host}")
    for job_id, job in listed.items():
        if job_id in known:
            out.append(job)
            continue
        if fetched >= MAX_NEW_DETAILS:
            deferred += 1
            continue
        fetched += 1
        try:
            res = await client.get(job.apply_url)
        except FetchError as e:
            if e.status in (404, 410):
                continue
            raise
        try:
            out.append(parse_job(res.content.decode("utf-8", "replace"), job))
            reads.ok()
        except ParseError as e:
            reads.skip(job.apply_url, e)
    if deferred:
        log.info("successfactors %s: %d new jobs left for the next night (cap %d)", host, deferred, MAX_NEW_DETAILS)
    return out


def _date(value: str | None) -> datetime | None:
    value = re.sub(r"\bSept\b", "Sep", (value or "").strip())  # "28 Sept 2026"
    for fmt in ("%b %d, %Y", "%d %b %Y", "%d-%b-%Y"):
        try:
            return datetime.strptime(value, fmt).replace(tzinfo=timezone.utc)
        except ValueError:
            continue
    return None
