"""What happens to each job after one crawl of one company.

Pure logic, no database: existing jobs + tonight's result in, changes out.

Rules (spec: "What happens to each job"):
- New key -> insert.
- Key seen again -> update fields and last_seen, reset missed_runs.
- Closed key seen again -> reopen, keeping first_seen.
- Open key missing on two consecutive *successful* crawls -> close.
  Failed crawls never call this module, so outages never close jobs.
- Sanity check: a company that had MIN_JOBS_FOR_SANITY or more open jobs and
  now returns 0, or loses more than MAX_LOSS_RATIO of them at once, is treated
  as a broken page: change nothing, flag it.
"""

from dataclasses import dataclass, field

from adapters.base import RawJob

CLOSE_AFTER_MISSES = 2
MIN_JOBS_FOR_SANITY = 5
MAX_LOSS_RATIO = 0.8


@dataclass(frozen=True)
class ExistingJob:
    id: int
    job_key: str
    status: str  # "open" | "closed"
    missed_runs: int


@dataclass
class Diff:
    inserts: list[tuple[str, RawJob]] = field(default_factory=list)  # (job_key, job)
    updates: list[tuple[int, RawJob]] = field(default_factory=list)  # open and seen again
    reopens: list[tuple[int, RawJob]] = field(default_factory=list)  # closed and seen again
    touch: list[int] = field(default_factory=list)  # seen on a 304: only last_seen moves
    misses: list[tuple[int, int]] = field(default_factory=list)  # (id, new missed_runs)
    closes: list[int] = field(default_factory=list)
    broken_reason: str | None = None

    @property
    def broken(self) -> bool:
        return self.broken_reason is not None


def diff(existing: list[ExistingJob], fetched: list[tuple[str, RawJob]]) -> Diff:
    """Tonight's full job list (200 response) against the company's jobs in the DB."""
    seen: dict[str, RawJob] = {}
    for key, job in fetched:
        seen.setdefault(key, job)  # the same job listed twice counts once

    by_key = {e.job_key: e for e in existing}
    open_jobs = [e for e in existing if e.status == "open"]

    reason = _sanity(open_jobs, seen)
    if reason:
        return Diff(broken_reason=reason)

    d = Diff()
    for key, job in seen.items():
        e = by_key.get(key)
        if e is None:
            d.inserts.append((key, job))
        elif e.status == "open":
            d.updates.append((e.id, job))
        else:
            d.reopens.append((e.id, job))
    for e in open_jobs:
        if e.job_key not in seen:
            _miss(d, e)
    return d


def diff_not_modified(existing: list[ExistingJob]) -> Diff:
    """A 304: the list is identical to the last successful crawl. Jobs present
    then are present now; jobs already missing then are missing again."""
    d = Diff()
    for e in existing:
        if e.status != "open":
            continue
        if e.missed_runs == 0:
            d.touch.append(e.id)
        else:
            _miss(d, e)
    return d


def _miss(d: Diff, e: ExistingJob) -> None:
    missed = e.missed_runs + 1
    if missed >= CLOSE_AFTER_MISSES:
        d.closes.append(e.id)
    else:
        d.misses.append((e.id, missed))


def _sanity(open_jobs: list[ExistingJob], seen: dict[str, RawJob]) -> str | None:
    before = len(open_jobs)
    if before < MIN_JOBS_FOR_SANITY:
        return None
    if not seen:
        return f"0 jobs returned; {before} were open"
    lost = sum(1 for e in open_jobs if e.job_key not in seen)
    if lost / before > MAX_LOSS_RATIO:
        return f"{lost} of {before} open jobs missing at once (>{MAX_LOSS_RATIO:.0%})"
    return None
