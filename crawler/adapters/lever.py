"""Lever postings API.

GET https://api.lever.co/v0/postings/{slug}?mode=json
(EU accounts: api.eu.lever.co, registered as "lever_eu".)
One call returns every published posting.
"""

import html
from datetime import datetime, timezone

from adapters.base import ParseError, RawJob
from clean import annual_inr, cities_from, clean_html, work_mode_from

NAME = "lever"
_PERIODS = {"per-year-salary": "year", "per-month-salary": "month"}


def board_url(slug: str) -> str:
    return f"https://api.lever.co/v0/postings/{slug}?mode=json"


def board_url_eu(slug: str) -> str:
    return f"https://api.eu.lever.co/v0/postings/{slug}?mode=json"


def parse(data, slug: str) -> list[RawJob]:
    if not isinstance(data, list):
        raise ParseError("lever: expected a list of postings")
    jobs = []
    for p in data:
        if p.get("id") in (None, "") or not p.get("text") or not p.get("hostedUrl"):
            continue
        cats = p.get("categories") or {}
        locations = cats.get("allLocations") or [cats.get("location") or ""]
        mode = p.get("workplaceType")
        if mode not in ("remote", "hybrid", "onsite"):
            mode = work_mode_from(locations)
        salary = p.get("salaryRange") or {}
        lo, hi = annual_inr(salary.get("min"), salary.get("max"), salary.get("currency"),
                            _PERIODS.get(salary.get("interval")))
        jobs.append(
            RawJob(
                source_job_id=str(p["id"]),
                title=p["text"].strip(),
                apply_url=p["hostedUrl"],
                description=clean_html(_description(p)),
                cities=cities_from(locations),
                work_mode=mode,
                salary_min=lo,
                salary_max=hi,
                posted_at=_ms(p.get("createdAt")),
            )
        )
    return jobs


def _description(p) -> str:
    parts = [p.get("description") or (p.get("opening", "") + p.get("descriptionBody", ""))]
    for lst in p.get("lists") or []:
        heading = (lst.get("text") or "").strip()
        if heading:
            parts.append(f"<h3>{html.escape(heading)}</h3>")
        parts.append(f"<ul>{lst.get('content') or ''}</ul>")
    parts.append(p.get("additional") or "")
    return "".join(parts)


def _ms(value) -> datetime | None:
    try:
        return datetime.fromtimestamp(int(value) / 1000, tz=timezone.utc) if value else None
    except (TypeError, ValueError, OverflowError):
        return None
