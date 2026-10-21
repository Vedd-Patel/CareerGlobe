"""Amazon adapter against a saved real response (trimmed, fetched 2026-10-22)."""

import json
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, amazon, detect
from fetcher import PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"


def data():
    return json.loads((FIXTURES / "amazon_india.json").read_text())


def test_parse_india_page():
    jobs = amazon.parse(data())
    assert [j.source_job_id for j in jobs] == ["10585094", "10584954", "10582440"]
    j = jobs[0]
    assert j.title.startswith("Ops Support Expert")
    assert j.apply_url == "https://www.amazon.jobs/en/jobs/10585094/ops-support-expert-nasc-north-america-supply-chain-nasc"
    assert j.cities == ["Bengaluru"] and j.work_mode is None
    assert j.posted_at == datetime(2026, 10, 21, tzinfo=timezone.utc)
    assert "<h3>Basic qualifications</h3>" in j.description
    assert jobs[1].cities == ["Hyderabad"]


def test_virtual_multi_location_job_is_remote_with_all_cities():
    j = amazon.parse(data())[2]
    assert j.cities == ["Bengaluru", "Mumbai", "Gurugram"]
    assert j.work_mode == "remote"


def test_non_india_locations_are_dropped():
    job = {"id_icims": "1", "title": "SDE", "job_path": "/en/jobs/1/sde", "city": "Seattle",
           "locations": [json.dumps({"countryIso2a": "US", "city": "Seattle"}),
                         json.dumps({"countryIso2a": "IN", "normalizedCityName": "Chennai"})]}
    assert amazon.parse({"jobs": [job]})[0].cities == ["Chennai"]


@pytest.mark.parametrize("bad", [[], {"jobs": None}, {"error": "boom", "jobs": []}])
def test_unexpected_shape_raises(bad):
    with pytest.raises(ParseError):
        amazon.parse(bad)


def test_detect():
    assert detect("https://www.amazon.jobs/en/") == ("amazon", "amazon")


class FakeAmazon:
    def __init__(self, n):
        self.n, self.offsets = n, []

    def __call__(self, request):
        if request.url.path == "/robots.txt":
            return httpx.Response(200, text="User-agent: *\nDisallow: /internal\n")
        off, lim = int(request.url.params["offset"]), int(request.url.params["result_limit"])
        assert request.url.params["normalized_country_code[]"] == "IND"
        self.offsets.append(off)
        ids = range(off, min(off + lim, self.n))
        return httpx.Response(200, json={"error": None, "hits": self.n, "jobs": [
            {"id_icims": str(i), "title": f"Job {i}", "job_path": f"/en/jobs/{i}/job", "city": "Pune"} for i in ids]})


@pytest.mark.anyio
async def test_fetch_pages_until_hits():
    server = FakeAmazon(250)
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(server)) as c:
        jobs = await amazon.fetch(c, "amazon", {})
    assert server.offsets == [0, 100, 200] and len(jobs) == 250


@pytest.mark.anyio
async def test_fetch_refuses_a_list_that_may_be_cut_off(monkeypatch):
    monkeypatch.setattr(amazon, "MAX_PAGES", 2)
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(FakeAmazon(500))) as c:
        with pytest.raises(ParseError, match="cut off"):
            await amazon.fetch(c, "amazon", {})
