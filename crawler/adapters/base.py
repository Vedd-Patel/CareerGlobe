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
    # Seen in a list without its detail page (only for jobs already in the DB):
    # title, link and (if given) posted_at are refreshed; stored details are kept.
    partial: bool = False
    # The source's own change stamp (sitemap <lastmod>); a job page is fetched
    # again only when it changes.
    source_updated: str | None = None
    # Set by adapters whose boards are worldwide (Greenhouse, Lever, Keka, Workable):
    # False = clearly outside India, so the crawl drops it.
    in_india: bool | None = None


class ParseError(Exception):
    """The response did not have the shape the adapter expects (format change)."""
