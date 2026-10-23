"""Oracle Recruiting Cloud against saved real responses (Zensar, trimmed, fetched 2026-10-22)."""

import json
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, detect, oracle
from fetcher import PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"
SLUG = "fa-etvl-saasfaprod1.fa.ocs.oraclecloud.com|CX_1"


def load(name):
    return json.loads((FIXTURES / name).read_text())


def test_list_keeps_india_only():
    jobs, total, rows = oracle.parse_list(load("oracle_zensar_list.json"), SLUG)
    assert (total, rows) == (194, 4)
    assert [j.source_job_id for j in jobs] == ["145497", "145943", "148032"]  # the Geneva job is dropped
    j = jobs[1]
    assert j.cities == ["Hyderabad"] and j.partial
    assert j.apply_url == ("https://fa-etvl-saasfaprod1.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience"
                           "/en/sites/CX_1/job/145943")
    assert jobs[0].cities == []  # "India": no city guessed


def test_detail_adds_city_and_description():
    listed = oracle.parse_list(load("oracle_zensar_list.json"), SLUG)[0][0]
    j = oracle.parse_detail(load("oracle_zensar_detail.json"), listed)
    assert j.cities == ["Pune"]  # from workLocation.TownOrCity
    assert j.description.startswith("<p>5+ Years of GW Configuration experience")
    assert j.description.count("Guidewire BillingCenter Developer") == 1  # identical sections kept once
    assert j.posted_at == datetime(2026, 10, 21, 17, 8, 38, tzinfo=timezone.utc)
    assert not j.partial


@pytest.mark.parametrize("bad", [{}, {"items": []}, {"items": [{}]}])
def test_unexpected_shape_raises(bad):
    with pytest.raises(ParseError):
        oracle.parse_list(bad, SLUG)


@pytest.mark.parametrize("url, want", [
    ("https://iaaviz.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience/en/sites/Jobs-at-Icertis/",
     ("oracle", "iaaviz.fa.ocs.oraclecloud.com|Jobs-at-Icertis")),
    ("https://fa-etvl-saasfaprod1.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience/en/sites/CX_1/job/145497",
     ("oracle", SLUG)),
    ("https://iaaviz.fa.ocs.oraclecloud.com/hcmUI/CandidateExperience", None),
])
def test_detect(url, want):
    assert detect(url) == want


class FakeOracle:
    def __init__(self, n):
        self.n, self.details = n, []

    def __call__(self, request):
        path = request.url.path
        if path == "/robots.txt":
            return httpx.Response(404)
        if path.endswith("/recruitingCEJobRequisitions"):
            finder = request.url.params["finder"]
            self.finders = getattr(self, "finders", []) + [finder]
            offset = int(finder.split("offset=")[1].split(",")[0])
            rows = [{"Id": str(i), "Title": f"Role {i}", "PrimaryLocationCountry": "IN" if i % 2 == 0 else "US",
                     "PrimaryLocation": "Pune, Maharashtra, India"} for i in range(offset, min(offset + 25, self.n))]
            return httpx.Response(200, json={"items": [{"TotalJobsCount": self.n, "requisitionList": rows}]})
        if path.endswith("/recruitingCEJobRequisitionDetails"):
            self.details.append(request.url.params["finder"])
            return httpx.Response(200, json={"items": [{"Title": "Role", "ExternalDescriptionStr": "<p>x</p>",
                                                        "workLocation": [{"TownOrCity": "Pune", "Country": "IN"}]}]})
        return httpx.Response(404)


@pytest.mark.anyio
async def test_fetch_pages_and_details_for_new_india_jobs(monkeypatch):
    monkeypatch.setattr(oracle, "MIN_GAP", 0.0)
    server = FakeOracle(60)
    async with PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                            transport=httpx.MockTransport(server)) as c:
        jobs = await oracle.fetch(c, SLUG, {"0": None})
    assert len(jobs) == 30  # even IDs are in India
    assert len(server.details) == 29
    assert server.details[0] == 'ById;Id="2",siteNumber=CX_1'


def test_india_location_facet_and_url():
    data = {"items": [{"locationsFacet": [{"Id": 5, "Name": "United States"}, {"Id": 300000000295773, "Name": "India"}]}]}
    assert oracle.india_location(data) == 300000000295773
    assert oracle.india_location({"items": [{}]}) is None
    url = oracle.list_url(SLUG, 25, india=300000000295773)
    assert "selectedLocationsFacet=300000000295773" in url and "offset=25" in url
    assert "facetsList=LOCATIONS" in oracle.list_url(SLUG, 0, facets=True)
