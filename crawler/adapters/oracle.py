"""Oracle Recruiting Cloud (Oracle HCM "Candidate Experience") career sites.

List:   GET https://{host}/hcmRestApi/resources/latest/recruitingCEJobRequisitions
        ?onlyData=true&expand=requisitionList.secondaryLocations
        &finder=findReqs;siteNumber={site},limit=25,offset={n},sortBy=POSTING_DATES_DESC
Detail: GET https://{host}/hcmRestApi/resources/latest/recruitingCEJobRequisitionDetails
        ?expand=all&onlyData=true&finder=ById;Id="{id}",siteNumber={site}

Undocumented for candidates but the calls the careers site makes. The
*.fa.ocs.oraclecloud.com hosts checked on 2026-10-22 have no robots.txt (404:
no rules). Seed slug: "{host}|{site}" from a careers URL like
https://{host}/hcmUI/CandidateExperience/en/sites/{site}.

- Only jobs whose primary or a secondary location is in India.
- The list gives title, date and a location string; the city and the
  description come from the detail call, made only for jobs new to us.
"""

import logging
import re
from datetime import datetime, timezone

from adapters.base import DetailReads, ParseError, RawJob
from clean import cities_from, clean_html, work_mode_from
from fetcher import FetchError

log = logging.getLogger(__name__)
NAME = "oracle"
PAGE_SIZE = 25
MAX_PAGES = 200
MAX_NEW_DETAILS = 300
_SLUG = re.compile(r"^([a-z0-9.-]+\.oraclecloud\.com)\|([A-Za-z0-9_-]+)$")
API = "https://{host}/hcmRestApi/resources/latest"


def parse_slug(slug: str) -> tuple[str, str]:
    m = _SLUG.match(slug or "")
    if not m:
        raise ParseError(f"oracle: bad slug {slug!r} (want host|site)")
    return m.group(1), m.group(2)


def list_url(slug: str, offset: int) -> str:
    host, site = parse_slug(slug)
    return (f"{API.format(host=host)}/recruitingCEJobRequisitions?onlyData=true"
            f"&expand=requisitionList.secondaryLocations"
            f"&finder=findReqs;siteNumber={site},limit={PAGE_SIZE},offset={offset},sortBy=POSTING_DATES_DESC")


def detail_url(slug: str, job_id: str) -> str:
    host, site = parse_slug(slug)
    return (f"{API.format(host=host)}/recruitingCEJobRequisitionDetails?expand=all&onlyData=true"
            f'&finder=ById;Id=%22{job_id}%22,siteNumber={site}')


def job_url(slug: str, job_id: str) -> str:
    host, site = parse_slug(slug)
    return f"https://{host}/hcmUI/CandidateExperience/en/sites/{site}/job/{job_id}"


def parse_list(data, slug: str) -> tuple[list[RawJob], int, int]:
    """(India jobs on this page as partial jobs, total jobs on the site, rows on this page)."""
    items = data.get("items") if isinstance(data, dict) else None
    if not isinstance(items, list) or not items or not isinstance(items[0].get("requisitionList"), list):
        raise ParseError("oracle: expected items[0].requisitionList")
    page = items[0]["requisitionList"]
    jobs = []
    for r in page:
        job_id, title = str(r.get("Id") or ""), (r.get("Title") or "").strip()
        secondary = [s for s in r.get("secondaryLocations") or [] if isinstance(s, dict)]
        in_india = r.get("PrimaryLocationCountry") == "IN" or any(s.get("CountryCode") == "IN" for s in secondary)
        if not job_id or not title or not in_india:
            continue
        places = [r.get("PrimaryLocation") or ""] + [s.get("Name") or "" for s in secondary if s.get("CountryCode") == "IN"]
        jobs.append(RawJob(
            source_job_id=job_id,
            title=title,
            apply_url=job_url(slug, job_id),
            cities=cities_from(places),
            work_mode=work_mode_from(places),
            posted_at=_dt(r.get("PostedDate")),
            partial=True,
        ))
    return jobs, int(items[0].get("TotalJobsCount") or 0), len(page)


def parse_detail(data, listed: RawJob) -> RawJob:
    items = data.get("items") if isinstance(data, dict) else None
    if not isinstance(items, list) or not items:
        raise ParseError(f"oracle: no detail for job {listed.source_job_id}")
    r = items[0]
    parts, seen = [], set()
    for key in ("ExternalDescriptionStr", "ExternalResponsibilitiesStr", "ExternalQualificationsStr"):
        text = (r.get(key) or "").strip()
        bare = re.sub(r"<[^>]*>|&nbsp;|\s", "", text)  # sections often repeat with only markup changes
        if bare and bare not in seen:
            seen.add(bare)
            parts.append(text)
    towns = [w.get("TownOrCity") or "" for w in r.get("workLocation") or [] if w.get("Country") in (None, "IN")]
    cities = cities_from(towns) or listed.cities
    return RawJob(
        source_job_id=listed.source_job_id,
        title=(r.get("Title") or "").strip() or listed.title,
        apply_url=listed.apply_url,
        description=clean_html("".join(parts)),
        cities=cities,
        work_mode=work_mode_from([r.get("WorkplaceType") or "", r.get("PrimaryLocation") or ""]) or listed.work_mode,
        posted_at=_dt(r.get("ExternalPostedStartDate")) or listed.posted_at,
    )


async def fetch(client, slug: str, known) -> list[RawJob]:
    listed: dict[str, RawJob] = {}
    total = None
    for page in range(MAX_PAGES):
        res = await client.get(list_url(slug, page * PAGE_SIZE))
        try:
            jobs, count, rows = parse_list(res.json(), slug)
        except ValueError as e:
            raise ParseError(f"oracle: list is not JSON: {e}") from e
        total = count if total is None else total
        for j in jobs:
            listed.setdefault(j.source_job_id, j)
        if rows < PAGE_SIZE or (page + 1) * PAGE_SIZE >= total:
            break
    else:
        raise ParseError(f"oracle: more than {MAX_PAGES} pages; list may be cut off")

    out, fetched, deferred = [], 0, 0
    reads = DetailReads(f"oracle {slug}")
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
            reads.ok()
        except (ValueError, ParseError) as e:
            reads.skip(job_id, e)
    if deferred:
        log.info("oracle %s: %d new jobs left for the next night (cap %d)", slug, deferred, MAX_NEW_DETAILS)
    return out


def _dt(value) -> datetime | None:
    if not value:
        return None
    try:
        d = datetime.fromisoformat(str(value).replace("Z", "+00:00"))
    except ValueError:
        return None
    return d if d.tzinfo else d.replace(tzinfo=timezone.utc)
