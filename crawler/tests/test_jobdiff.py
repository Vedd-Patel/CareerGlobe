"""The closing rule and sanity check, night by night.

`Board` keeps jobs in memory the way the DB will, so each test reads like a
sequence of nights. The Phase 1 "done when" scenarios are the first three tests.
"""

from adapters.base import RawJob
from jobdiff import Diff, ExistingJob, diff, diff_not_modified


def job(key):
    return key, RawJob(source_job_id=key, title=f"Job {key}", apply_url=f"https://x.com/{key}")


class Board:
    def __init__(self):
        self.rows: dict[str, dict] = {}
        self.flags: list[str] = []
        self.night = 0

    def existing(self):
        return [ExistingJob(r["id"], k, r["status"], r["missed"]) for k, r in self.rows.items()]

    def _by_id(self, i):
        return next(r for r in self.rows.values() if r["id"] == i)

    def crawl(self, keys) -> Diff:
        """A successful crawl returning these job keys."""
        return self._apply(diff(self.existing(), [job(k) for k in keys]))

    def crawl_304(self) -> Diff:
        return self._apply(diff_not_modified(self.existing()))

    def crawl_failed(self):
        """Timeout / block / error: the crawler never calls the diff."""
        self.night += 1

    def _apply(self, d: Diff) -> Diff:
        self.night += 1
        if d.broken:
            self.flags.append(d.broken_reason)
            return d
        for key, _ in d.inserts:
            self.rows[key] = {"id": len(self.rows) + 1, "status": "open", "missed": 0,
                              "first_seen": self.night, "last_seen": self.night}
        for i, _ in d.updates + d.reopens:
            r = self._by_id(i)
            r.update(status="open", missed=0, last_seen=self.night)
        for i in d.touch:
            self._by_id(i)["last_seen"] = self.night
        for i, missed in d.misses:
            self._by_id(i)["missed"] = missed
        for i in d.closes:
            self._by_id(i).update(status="closed", missed=2)
        return d

    def open_keys(self):
        return sorted(k for k, r in self.rows.items() if r["status"] == "open")


ALL = [f"j{i}" for i in range(10)]


# --- Phase 1 "done when" -------------------------------------------------

def test_two_runs_in_a_row_close_nothing():
    b = Board()
    first = b.crawl(ALL)
    second = b.crawl(ALL)
    assert len(first.inserts) == 10
    assert not second.inserts and not second.closes and not second.misses
    assert len(second.updates) == 10
    assert b.open_keys() == sorted(ALL)


def test_simulated_outage_closes_nothing():
    b = Board()
    b.crawl(ALL)
    for _ in range(5):  # a week of failures
        b.crawl_failed()
    assert b.open_keys() == sorted(ALL)
    assert all(r["missed"] == 0 for r in b.rows.values())


def test_removed_job_closes_after_two_good_runs():
    b = Board()
    b.crawl(ALL)
    without = [k for k in ALL if k != "j3"]

    d1 = b.crawl(without)
    assert d1.misses == [(b.rows["j3"]["id"], 1)] and not d1.closes
    assert "j3" in b.open_keys()  # one miss is not enough

    d2 = b.crawl(without)
    assert d2.closes == [b.rows["j3"]["id"]]
    assert "j3" not in b.open_keys()


# --- the two-night rule in detail -----------------------------------------

def test_failed_night_between_misses_does_not_reset_or_count():
    b = Board()
    b.crawl(ALL)
    without = [k for k in ALL if k != "j3"]
    b.crawl(without)  # miss 1
    b.crawl_failed()  # neither a miss nor a sighting
    assert b.rows["j3"]["missed"] == 1
    b.crawl(without)  # miss 2 on the next *successful* crawl
    assert "j3" not in b.open_keys()


def test_job_that_comes_back_after_one_miss_stays_open_and_resets():
    b = Board()
    b.crawl(ALL)
    b.crawl([k for k in ALL if k != "j3"])
    b.crawl(ALL)
    assert b.rows["j3"]["missed"] == 0
    b.crawl([k for k in ALL if k != "j3"])  # a single miss again: still open
    assert "j3" in b.open_keys()


def test_closed_job_reappearing_is_reopened_keeping_first_seen():
    b = Board()
    b.crawl(ALL)
    without = [k for k in ALL if k != "j3"]
    b.crawl(without)
    b.crawl(without)
    assert "j3" not in b.open_keys()

    d = b.crawl(ALL)
    assert [i for i, _ in d.reopens] == [b.rows["j3"]["id"]]
    assert not d.inserts
    assert b.rows["j3"]["first_seen"] == 1 and b.rows["j3"]["status"] == "open"


def test_new_jobs_are_inserted_alongside_existing():
    b = Board()
    b.crawl(ALL)
    d = b.crawl(ALL + ["new1", "new2"])
    assert [k for k, _ in d.inserts] == ["new1", "new2"]


def test_duplicate_job_in_one_response_counts_once():
    d = diff([], [job("a"), job("a"), job("b")])
    assert [k for k, _ in d.inserts] == ["a", "b"]


# --- 304 Not Modified ------------------------------------------------------

def test_304_keeps_present_jobs_open():
    b = Board()
    b.crawl(ALL)
    d = b.crawl_304()
    assert len(d.touch) == 10 and not d.closes
    assert b.open_keys() == sorted(ALL)


def test_304_after_a_miss_counts_as_the_second_miss():
    b = Board()
    b.crawl(ALL)
    b.crawl([k for k in ALL if k != "j3"])  # miss 1
    d = b.crawl_304()  # same list as last night: j3 is still absent
    assert d.closes == [b.rows["j3"]["id"]]
    assert "j3" not in b.open_keys()


# --- sanity check ------------------------------------------------------------

def test_drop_to_zero_is_flagged_and_closes_nothing():
    b = Board()
    b.crawl(ALL)
    d = b.crawl([])
    assert d.broken and "0 jobs returned; 10 were open" in d.broken_reason
    assert not (d.inserts or d.updates or d.misses or d.closes)
    assert b.open_keys() == sorted(ALL)
    # and it did not count as a miss either
    assert all(r["missed"] == 0 for r in b.rows.values())


def test_losing_more_than_80_percent_is_flagged():
    b = Board()
    b.crawl(ALL)
    d = b.crawl(["j0"])  # 9 of 10 gone
    assert d.broken and "9 of 10" in d.broken_reason
    assert b.open_keys() == sorted(ALL)


def test_losing_exactly_80_percent_is_allowed():
    b = Board()
    b.crawl(ALL)
    d = b.crawl(["j0", "j1"])  # 8 of 10 gone: not more than 80%
    assert not d.broken
    assert len(d.misses) == 8


def test_broken_page_with_new_junk_jobs_is_still_flagged():
    # A redesign that returns 10 unrelated "jobs" (e.g. nav links) loses all the real ones.
    b = Board()
    b.crawl(ALL)
    d = b.crawl([f"junk{i}" for i in range(10)])
    assert d.broken
    assert not any(k.startswith("junk") for k in b.rows)


def test_small_companies_below_five_jobs_can_close_everything():
    b = Board()
    b.crawl(["a", "b", "c", "d"])
    b.crawl([])
    b.crawl([])
    assert b.open_keys() == [] and not b.flags


def test_exactly_five_open_jobs_triggers_the_check():
    b = Board()
    b.crawl(["a", "b", "c", "d", "e"])
    assert b.crawl([]).broken


def test_closed_jobs_do_not_count_toward_the_threshold():
    b = Board()
    b.crawl(ALL)
    keep = ["j0", "j1", "j2"]
    b.crawl(keep)  # 7 of 10 lost: allowed, miss 1
    b.crawl(keep)  # miss 2: 7 closed
    assert b.open_keys() == keep
    d = b.crawl([])  # 3 open now, below 5: allowed
    assert not d.broken
