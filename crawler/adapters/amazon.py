"""Amazon (amazon.jobs).

GET https://www.amazon.jobs/en/search.json?normalized_country_code[]=IND
    &result_limit=100&offset={n}&sort=recent
Undocumented: the call amazon.jobs' own search page makes. robots.txt
(checked 2026-10-21/22) disallows only /internal paths. At most 100 a page
(larger limits return no jobs); descriptions come with the list, so there is
one request per 100 jobs and no per-job requests. Seed slug: "amazon".
"""

import json
import re
from datetime import datetime, timezone

from adapters.base import ParseError, RawJob
from clean import cities_from, clean_html, work_mode_from

NAME = "amazon"
PAGE_SIZE = 100
MAX_PAGES = 100
SEARCH = ("https://www.amazon.jobs/en/search.json?normalized_country_code%5B%5D=IND"
          "&result_limit={limit}&offset={offset}&sort=recent")


def page_url(offset: int) -> str:
    return SEARCH.format(limit=PAGE_SIZE, offset=offset)


def parse(data, slug: str = "amazon") -> list[RawJob]:
    if not isinstance(data, dict) or not isinstance(data.get("jobs"), list):
        raise ParseError("amazon: expected an object with a 'jobs' list")
    if data.get("error"):
        raise ParseError(f"amazon: search error {data['error']!r}")
    jobs = []
    for j in data["jobs"]:
        job_id, title, path = j.get("id_icims"), (j.get("title") or "").strip(), j.get("job_path")
        if not job_id or not title or not path:
            continue
        places, virtual = _locations(j)
        jobs.append(RawJob(
            source_job_id=str(job_id),
            title=title,
            apply_url=f"https://www.amazon.jobs{path}",
            description=clean_html(_description(j)),
            cities=cities_from(places),
            work_mode="remote" if virtual else work_mode_from(places),
            posted_at=_date(j.get("posted_date")),
        ))
    return jobs


def _locations(j: dict) -> tuple[list[str], bool]:
    """India places from the structured `locations` list (JSON strings), else the
    flat fields. True when every India location is virtual (work from home)."""
    places, types = [], []
    for raw in j.get("locations") or []:
        try:
            loc = json.loads(raw) if isinstance(raw, str) else raw
        except ValueError:
            continue
        if not isinstance(loc, dict) or loc.get("countryIso2a") not in (None, "IN"):
            continue
        places.append(loc.get("normalizedCityName") or re.sub(r"\s*-\s*Virtual$", "", loc.get("city") or ""))
        types.append(loc.get("type"))
    if not places and j.get("city"):
        places = [j["city"]]
    return places, bool(types) and all(t == "VIRTUAL" for t in types)


def _description(j: dict) -> str:
    parts = [f"<p>{j.get('description') or ''}</p>"]
    for key, heading in (("basic_qualifications", "Basic qualifications"),
                         ("preferred_qualifications", "Preferred qualifications")):
        if j.get(key):
            parts.append(f"<h3>{heading}</h3><p>{j[key]}</p>")
    return "".join(parts)


async def fetch(client, slug: str, known) -> list[RawJob]:
    """Every India job, page by page. `known` is unused: the list is complete."""
    jobs: dict[str, RawJob] = {}
    total = None
    for page in range(MAX_PAGES):
        res = await client.get(page_url(page * PAGE_SIZE))
        try:
            data = res.json()
        except ValueError as e:
            raise ParseError(f"amazon: not JSON: {e}") from e
        batch = parse(data, slug)
        if total is None:
            total = int(data.get("hits") or 0)
        for job in batch:
            jobs.setdefault(job.source_job_id, job)  # "recent" order can shift while paging
        if len(data["jobs"]) < PAGE_SIZE or (page + 1) * PAGE_SIZE >= total:
            return list(jobs.values())
    raise ParseError(f"amazon: more than {MAX_PAGES * PAGE_SIZE} India jobs; list may be cut off")


def _date(value: str | None) -> datetime | None:
    try:
        return datetime.strptime((value or "").strip(), "%B %d, %Y").replace(tzinfo=timezone.utc)
    except ValueError:
        return None
