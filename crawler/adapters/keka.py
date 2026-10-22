"""Keka careers portal.

GET https://{slug}.keka.com/careers/api/jobs/default/active
Undocumented: it is the call Keka's own careers page makes. robots.txt allows
/careers. No ETag, so the full list is downloaded each night (it is small).
Job page: https://{slug}.keka.com/careers/jobdetails/{id}
"""

from datetime import datetime

from adapters.base import ParseError, RawJob
from clean import annual_inr, cities_from, clean_html, in_india, work_mode_from

NAME = "keka"
# Keka's SalaryPeriod enum: 0 not available, 1 hourly, 2 bi-weekly, 3 monthly, 4 annual.
_PERIODS = {3: "month", 4: "year"}


def board_url(slug: str) -> str:
    return f"https://{slug}.keka.com/careers/api/jobs/default/active"


def parse(data, slug: str) -> list[RawJob]:
    if not isinstance(data, list):
        raise ParseError("keka: expected a list of jobs")
    jobs = []
    for j in data:
        if j.get("id") in (None, "") or not j.get("title"):
            continue
        locs = j.get("jobLocations") or []
        names = [loc.get("name") or "" for loc in locs]
        # `city` is the structured field; `name` is a free label like "Remote".
        cities = cities_from([loc.get("city") or loc.get("name") or "" for loc in locs])
        salary = j.get("salaryRange") or {}
        lo, hi = annual_inr(salary.get("minimum"), salary.get("maximum"), salary.get("currency"),
                            _PERIODS.get(salary.get("salaryPeriod")))
        jobs.append(
            RawJob(
                source_job_id=str(j["id"]),
                title=j["title"].strip(),
                apply_url=f"https://{slug}.keka.com/careers/jobdetails/{j['id']}",
                description=clean_html(j.get("description")),
                cities=cities,
                work_mode=work_mode_from(names),
                in_india=in_india([" ".join(filter(None, [l.get("city"), l.get("state"), l.get("country"), l.get("name")])) for l in locs]),
                salary_min=lo,
                salary_max=hi,
                posted_at=_dt(j.get("publishedOn")),
            )
        )
    return jobs


def _dt(value: str | None) -> datetime | None:
    try:
        return datetime.fromisoformat(value) if value else None
    except ValueError:
        return None
