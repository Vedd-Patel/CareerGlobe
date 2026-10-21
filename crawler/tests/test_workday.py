"""Workday adapter: saved real NVIDIA responses (trimmed, fetched 2026-10-21)
and a fake Workday server for paging, per-job details and the safety limits."""

import json
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, workday
from fetcher import FetchError, PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"
SLUG = "nvidia.wd5/NVIDIAExternalCareerSite"
API = "/wday/cxs/nvidia/NVIDIAExternalCareerSite"
INDIA = "c4f78be1a8f14da0ab49ce1162348a5e"


def load(name):
    return json.loads((FIXTURES / name).read_text())


def test_detail_single_location():
    path = "/job/India-Pune/SWQA-Test-Developer-Engineer_JR2028109"
    j = workday.parse_detail(load("workday_nvidia_detail_single.json"), SLUG, path)
    assert j.source_job_id == "JR2028109"
    assert j.title == "SWQA Test Developer Engineer"
    assert j.apply_url == "https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite" + path
    assert j.cities == ["Pune"]
    assert j.work_mode is None
    assert j.salary_min is None and j.salary_max is None
    assert j.posted_at == datetime(2026, 10, 19, tzinfo=timezone.utc)
    assert j.description.startswith("<p>NVIDIA has advanced computer graphics")
    assert 'style="' not in j.description
    assert not j.partial


def test_detail_multi_location_matches_workdays_own_url():
    data = load("workday_nvidia_detail_multi.json")
    path = "/job/India-Pune/Senior-System-Software-Engineer---AI-SDK-Multimedia-platform_JR2027371-1"
    j = workday.parse_detail(data, SLUG, path)
    assert j.source_job_id == "JR2027371-1"  # one req, several postings: keep the -1
    assert j.cities == ["Pune", "Bengaluru"]
    assert j.apply_url == data["jobPostingInfo"]["externalUrl"]


def test_list_jobs_from_fixture_are_partial():
    postings = load("workday_nvidia_list.json")["jobPostings"]
    jobs = [workday.list_job(p, SLUG) for p in postings]
    assert [j.source_job_id for j in jobs] == ["JR2028109", "JR2027371-1", "JR2027806", "JR2014480-1"]
    assert all(j.partial and j.description is None for j in jobs)
    assert jobs[3].title == "CAD Engineer"


def test_india_facet_prefers_the_country_value():
    facets = load("workday_nvidia_list.json")["facets"]
    assert workday.india_facets(facets) == {"locationHierarchy1": ["2fcb99c455831013ea52b82135ba3266"]}


def test_india_facet_falls_back_to_locations_naming_india():
    facets = [{"facetParameter": "locationMainGroup", "values": [{"facetParameter": "locations", "values": [
        {"descriptor": "Bangalore, Karnataka, India", "id": "a"},
        {"descriptor": "Indianapolis, Indiana, United States of America", "id": "b"},
        {"descriptor": "Virtual India", "id": "c"},
        {"descriptor": "London, UK", "id": "d"},
    ]}]}]
    assert workday.india_facets(facets) == {"locations": ["a", "c"]}


def test_no_india_facet():
    facets = [{"facetParameter": "locations", "values": [{"descriptor": "Austin, TX", "id": "x"}]}]
    assert workday.india_facets(facets) == {}


@pytest.mark.parametrize("path, want", [
    ("/job/India-Pune/CAD-Engineer_JR2014480-1", "JR2014480-1"),
    ("/job/Bangalore/Senior_Data_Engineer_R0012345", "R0012345"),
    ("/job/Pune/No-Req-Id", "No-Req-Id"),
])
def test_posting_id(path, want):
    assert workday.posting_id(path) == want


@pytest.mark.parametrize("slug", ["nvidia/NVIDIAExternalCareerSite", "nvidia.wd5", "x.wd5/a/b"])
def test_bad_slug(slug):
    with pytest.raises(ParseError):
        workday.api_base(slug)


def test_detail_without_title_raises():
    with pytest.raises(ParseError):
        workday.parse_detail({"jobPostingInfo": {}}, SLUG, "/job/x_1")


# --- fetch() against a fake Workday site -------------------------------------------------


class FakeWorkday:
    def __init__(self, n_india=45, total=None, facets=True, gone=(), elsewhere=("US1", "US2")):
        self.ids = [f"R{i:04d}" for i in range(n_india)]
        self.elsewhere = list(elsewhere)
        self.total, self.facets, self.gone = total, facets, set(gone)
        self.list_bodies: list[dict] = []
        self.details: list[str] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path == "/robots.txt":
            return httpx.Response(200, text="User-agent: *\nDisallow: /refreshFacet/\n")
        if request.method == "POST" and path == f"{API}/jobs":
            body = json.loads(request.content)
            self.list_bodies.append(body)
            india = body["appliedFacets"] == {"locationCountry": [INDIA]}
            ids = self.ids if india else self.ids + self.elsewhere
            page = ids[body["offset"]:body["offset"] + body["limit"]]
            facets = [{"facetParameter": "locationCountry", "values": [
                {"descriptor": "United States of America", "id": "us", "count": 2},
                {"descriptor": "India", "id": INDIA, "count": len(self.ids)}]}] if self.facets else []
            total = self.total if self.total is not None else len(ids)
            return httpx.Response(200, json={
                "total": total if body["offset"] == 0 else 0,  # Workday reports 0 after page 1
                "jobPostings": [{"title": f"Engineer {i}", "externalPath": f"/job/India-Pune/Engineer_{i}",
                                 "locationsText": "India, Pune"} for i in page],
                "facets": facets})
        if request.method == "GET" and path.startswith(f"{API}/job/"):
            rid = path.rsplit("_", 1)[-1]
            self.details.append(rid)
            if rid in self.gone:
                return httpx.Response(404)
            return httpx.Response(200, json={"jobPostingInfo": {
                "title": f"Engineer {rid}", "jobDescription": "<p>Build</p>", "location": "India, Pune",
                "additionalLocations": ["India, Bengaluru"], "remoteType": "Hybrid", "startDate": "2026-10-01"}})
        return httpx.Response(404)


def client(server):
    return PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                        transport=httpx.MockTransport(server))


@pytest.mark.anyio
async def test_fetch_pages_through_india_and_details_only_new_jobs():
    server = FakeWorkday(n_india=45)
    known = {"R0000", "R0001"}
    async with client(server) as c:
        jobs = await workday.fetch(c, SLUG, known)

    # Probe page without filters, then 3 India pages of 20/20/5.
    assert [b["appliedFacets"] for b in server.list_bodies] == [{}] + [{"locationCountry": [INDIA]}] * 3
    assert [b["offset"] for b in server.list_bodies[1:]] == [0, 20, 40]
    assert len(jobs) == 45 and "US1" not in {j.source_job_id for j in jobs}
    assert len(server.details) == 43  # known jobs are not re-fetched
    by_id = {j.source_job_id: j for j in jobs}
    assert by_id["R0000"].partial and by_id["R0000"].title == "Engineer R0000"
    new = by_id["R0002"]
    assert not new.partial and new.description == "<p>Build</p>"
    assert new.cities == ["Pune", "Bengaluru"] and new.work_mode == "hybrid"


@pytest.mark.anyio
async def test_fetch_caps_new_details_per_night(monkeypatch):
    monkeypatch.setattr(workday, "MAX_NEW_DETAILS", 10)
    server = FakeWorkday(n_india=25)
    async with client(server) as c:
        jobs = await workday.fetch(c, SLUG, {"R0024"})
    assert len(server.details) == 10
    assert len(jobs) == 11  # 10 new with details + the known one; 14 wait for tomorrow


@pytest.mark.anyio
async def test_job_closed_between_list_and_detail_is_skipped():
    server = FakeWorkday(n_india=3, gone={"R0001"})
    async with client(server) as c:
        jobs = await workday.fetch(c, SLUG, set())
    assert [j.source_job_id for j in jobs] == ["R0000", "R0002"]


@pytest.mark.anyio
async def test_detail_server_error_fails_the_company():
    server = FakeWorkday(n_india=3)
    real = server.__call__

    def broken(request):
        if "/job/" in request.url.path:
            return httpx.Response(500)
        return real(request)

    async with client(broken) as c:
        with pytest.raises(FetchError):
            await workday.fetch(c, SLUG, set())


@pytest.mark.anyio
async def test_list_at_workdays_cap_fails_instead_of_closing_jobs():
    async with client(FakeWorkday(n_india=20, total=2000)) as c:
        with pytest.raises(ParseError, match="2000"):
            await workday.fetch(c, SLUG, set())


@pytest.mark.anyio
async def test_site_without_india_filter_fails():
    async with client(FakeWorkday(facets=False)) as c:
        with pytest.raises(ParseError, match="no India"):
            await workday.fetch(c, SLUG, set())


@pytest.mark.anyio
async def test_site_with_no_jobs_at_all_is_empty():
    async with client(FakeWorkday(n_india=0, facets=False, elsewhere=())) as c:
        assert await workday.fetch(c, SLUG, set()) == []


def test_india_facet_falls_back_to_bare_indian_city_names():
    facets = [{"facetParameter": "locationMainGroup", "values": [{"facetParameter": "locations", "values": [
        {"descriptor": "Amsterdam", "id": "a"}, {"descriptor": "Bengaluru", "id": "b"},
        {"descriptor": "Gurgaon", "id": "g"}, {"descriptor": "California", "id": "c"}]}]}]
    assert workday.india_facets(facets) == {"locations": ["b", "g"]}
