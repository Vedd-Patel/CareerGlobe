from dataclasses import dataclass, field
from datetime import datetime


@dataclass
class RawJob:
    """One job as read from a source, before it is matched against the DB."""

    source_job_id: str | None
    title: str
    apply_url: str
    description: str | None = None
    cities: list[str] = field(default_factory=list)
    work_mode: str | None = None
    salary_min: int | None = None
    salary_max: int | None = None
    posted_at: datetime | None = None


class ParseError(Exception):
    """The response did not have the shape the adapter expects (format change)."""
