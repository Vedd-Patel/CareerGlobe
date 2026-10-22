"""Workable (apply.workable.com) job boards.

GET https://apply.workable.com/api/v1/widget/accounts/{slug}?details=true
The public widget API: one call returns every published job with its
description and locations. robots.txt (checked 2026-10-22) allows everything.
Boards are worldwide: jobs are marked in_india from their country codes.
"""

from datetime import datetime, timezone

from adapters.base import ParseError, RawJob
from clean import cities_from, clean_html

NAME = "workable"


def board_url(slug: str) -> str:
    return f"https://apply.workable.com/api/v1/widget/accounts/{slug}?details=true"


def parse(data, slug: str) -> list[RawJob]:
    if not isinstance(data, dict) or not isinstance(data.get("jobs"), list):
        raise ParseError("workable: expected an object with a 'jobs' list")
    jobs = []
    for j in data["jobs"]:
        code, title, url = j.get("shortcode"), (j.get("title") or "").strip(), j.get("url") or j.get("shortlink")
        if not code or not title or not url:
            continue
        locs = [l for l in j.get("locations") or [] if isinstance(l, dict)]
        if not locs and (j.get("country") or j.get("city")):
            locs = [{"city": j.get("city"), "region": j.get("state"), "country": j.get("country")}]
        codes = {(l.get("countryCode") or "").upper() for l in locs} - {""}
        names = {(l.get("country") or "").lower() for l in locs} - {""}
        india = None
        if codes or names:
            india = "IN" in codes or "india" in names
        india_locs = [l for l in locs if (l.get("countryCode") or "").upper() == "IN" or (l.get("country") or "").lower() == "india"]
        remote = str(j.get("telecommuting")).lower() == "true"
        parts = [j.get("description") or "", j.get("requirements") or "", j.get("benefits") or ""]
        jobs.append(RawJob(
            source_job_id=str(code),
            title=title,
            apply_url=url,
            description=clean_html("".join(p for p in parts if p)),
            cities=cities_from([", ".join(filter(None, [l.get("city"), l.get("region")])) for l in india_locs]),
            work_mode="remote" if remote else None,
            posted_at=_date(j.get("published_on") or j.get("created_at")),
            in_india=india,
        ))
    return jobs


def _date(value: str | None) -> datetime | None:
    try:
        return datetime.fromisoformat(value).replace(tzinfo=timezone.utc) if value else None
    except ValueError:
        return None
