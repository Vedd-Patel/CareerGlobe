"""Greenhouse job board API.

GET https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true
One call returns every open job. `content` is HTML, entity-escaped.
"""

import html
from datetime import datetime

from adapters.base import ParseError, RawJob
from clean import cities_from, clean_html, work_mode_from

NAME = "greenhouse"


def board_url(slug: str) -> str:
    return f"https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true"


def parse(data, slug: str) -> list[RawJob]:
    if not isinstance(data, dict) or not isinstance(data.get("jobs"), list):
        raise ParseError("greenhouse: expected an object with a 'jobs' list")
    jobs = []
    for j in data["jobs"]:
        if not j.get("id") or not j.get("title") or not j.get("absolute_url"):
            continue
        locations = [(j.get("location") or {}).get("name") or ""]
        locations += [o.get("location") or o.get("name") or "" for o in j.get("offices") or []]
        content = j.get("content")
        jobs.append(
            RawJob(
                source_job_id=str(j["id"]),
                title=j["title"].strip(),
                apply_url=j["absolute_url"],
                description=clean_html(html.unescape(content)) if content else None,
                cities=cities_from(locations),
                work_mode=work_mode_from(locations),
                posted_at=_dt(j.get("first_published") or j.get("updated_at")),
            )
        )
    return jobs


def _dt(value: str | None) -> datetime | None:
    try:
        return datetime.fromisoformat(value) if value else None
    except ValueError:
        return None
