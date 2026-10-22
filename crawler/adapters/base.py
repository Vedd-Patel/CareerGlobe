import logging
from dataclasses import dataclass, field
from datetime import datetime

log = logging.getLogger(__name__)


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


class DetailReads:
    """Per-job detail pages: a few unreadable ones are skipped (the job is tried
    again the next night); many mean the format changed, and the company fails."""

    MIN_BAD, MAX_SHARE = 3, 0.2

    def __init__(self, source: str):
        self.source, self.read, self.bad = source, 0, 0

    def ok(self) -> None:
        self.read += 1

    def skip(self, what: str, err: Exception) -> None:
        self.read += 1
        self.bad += 1
        log.warning("%s: skipped unreadable detail %s (%s)", self.source, what, type(err).__name__)
        if self.bad >= self.MIN_BAD and self.bad / self.read > self.MAX_SHARE:
            raise ParseError(f"{self.source}: {self.bad} of {self.read} detail pages unreadable (last: {what}: {err})")
