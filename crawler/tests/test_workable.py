"""Workable widget API against a saved real response (Tiger Analytics, trimmed, 2026-10-22)."""

import json
from datetime import datetime, timezone
from pathlib import Path

import pytest

from adapters import ADAPTERS, ParseError, detect


def data():
    return json.loads((Path(__file__).parent / "fixtures" / "workable_tiger.json").read_text())


def test_parse_marks_india_and_foreign_jobs():
    jobs = ADAPTERS["workable"].parse(data(), "tiger-analytics")
    assert [j.source_job_id for j in jobs] == ["23D676B06C", "5EE9B8CE0F", "39D0D71D6D"]
    assert [j.in_india for j in jobs] == [True, True, False]
    j = jobs[0]
    assert j.title == "Global Immigration Specialist"
    assert j.apply_url == "https://apply.workable.com/j/23D676B06C"
    assert j.work_mode == "remote"
    assert isinstance(j.posted_at, datetime) and j.posted_at.tzinfo == timezone.utc
    assert j.description and "<script" not in j.description


def test_board_url_and_detect():
    assert ADAPTERS["workable"].board_url("acme") == "https://apply.workable.com/api/v1/widget/accounts/acme?details=true"
    assert detect("https://apply.workable.com/acme/") == ("workable", "acme")
    assert detect("https://apply.workable.com/j/39D0D71D6D") is None  # one job, not a board


@pytest.mark.parametrize("bad", [[], {"jobs": None}])
def test_unexpected_shape_raises(bad):
    with pytest.raises(ParseError):
        ADAPTERS["workable"].parse(bad, "x")
