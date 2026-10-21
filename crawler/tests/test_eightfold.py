"""Eightfold (Microsoft) against saved real responses (trimmed, fetched 2026-10-22)."""

import json
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, detect, eightfold as ef
from fetcher import PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"
SLUG = "apply.careers.microsoft.com|microsoft.com"


def load(name):
    return json.loads((FIXTURES / name).read_text())


def test_search_page():
    jobs, total = ef.parse_search(load("eightfold_microsoft_search.json"), SLUG)
    assert total == 232
    assert [j.source_job_id for j in jobs] == ["1970393557040609", "1970393556944049", "1970393556988781"]
    j = jobs[0]
    assert j.title == "Customer Success Account Manager"
    assert j.apply_url == "https://apply.careers.microsoft.com/careers/job/1970393557040609"
    assert j.cities == []  # "IN" only: no city guessed
    assert j.work_mode == "onsite" and j.partial
    assert j.posted_at == datetime.fromtimestamp(1791963855, tz=timezone.utc)
    assert jobs[2].cities == ["Hyderabad", "Noida"]


def test_detail_adds_description():
    listed = ef.parse_search(load("eightfold_microsoft_search.json"), SLUG)[0][0]
    j = ef.parse_detail(load("eightfold_microsoft_detail.json"), listed)
    assert not j.partial and j.description.startswith("<b>Overview</b>")
    assert j.apply_url == "https://apply.careers.microsoft.com/careers/job/1970393557040609"


def test_non_india_positions_are_dropped():
    data = {"data": {"count": 2, "positions": [
        {"id": 1, "name": "A", "standardizedLocations": ["Redmond, WA, US"]},
        {"id": 2, "name": "B", "standardizedLocations": ["Redmond, WA, US", "Pune, MH, IN"]}]}}
    jobs, _ = ef.parse_search(data, SLUG)
    assert [(j.source_job_id, j.cities) for j in jobs] == [("2", ["Pune"])]


@pytest.mark.parametrize("bad", [{}, {"data": {}}, []])
def test_unexpected_shape_raises(bad):
    with pytest.raises(ParseError):
        ef.parse_search(bad, SLUG)


@pytest.mark.parametrize("url, want", [
    ("https://jobs.careers.microsoft.com/global/en/search", ("eightfold", SLUG)),
    ("https://apply.careers.microsoft.com/careers", ("eightfold", SLUG)),
    ("https://acme.eightfold.ai/careers?domain=acme.com", ("eightfold", "acme.eightfold.ai|acme.com")),
    ("https://acme.eightfold.ai/careers", None),
])
def test_detect(url, want):
    assert detect(url) == want


class FakeEightfold:
    def __init__(self, n):
        self.n, self.details, self.searches = n, [], []

    def __call__(self, request):
        if request.url.path == "/robots.txt":
            return httpx.Response(200, text="User-agent: *\nDisallow: /\nAllow: /api/pcsx\nAllow: /careers\n")
        if request.url.path == "/api/pcsx/search":
            start = int(request.url.params["start"])
            self.searches.append(start)
            ids = range(start, min(start + 10, self.n))
            return httpx.Response(200, json={"data": {"count": self.n, "positions": [
                {"id": 100 + i, "name": f"Role {i}", "standardizedLocations": ["Pune, MH, IN"],
                 "positionUrl": f"/careers/job/{100 + i}"} for i in ids]}})
        if request.url.path == "/api/pcsx/position_details":
            self.details.append(request.url.params["position_id"])
            return httpx.Response(200, json={"data": {"jobDescription": "<p>Do it</p>"}})
        return httpx.Response(404)


@pytest.mark.anyio
async def test_fetch_pages_and_details_for_new_only(monkeypatch):
    monkeypatch.setattr(ef, "MIN_GAP", 0.0)
    server = FakeEightfold(25)
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(server)) as c:
        jobs = await ef.fetch(c, SLUG, {"100": None})
    assert server.searches == [0, 10, 20]
    assert len(jobs) == 25 and len(server.details) == 24
    assert next(j for j in jobs if j.source_job_id == "101").description == "<p>Do it</p>"


def test_adapter_asks_for_the_slow_gap():
    assert ef.MIN_GAP >= 5
