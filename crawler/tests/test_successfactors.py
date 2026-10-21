"""SuccessFactors classic career sites: saved real pages (trimmed, fetched
2026-10-21 from Tata Motors and Lupin) and a fake site for paging and limits."""

from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, successfactors as sf
from fetcher import FetchError, PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"


def read(name):
    return (FIXTURES / name).read_text()


def test_search_page_tatamotors():
    jobs, total = sf.parse_search(read("successfactors_tatamotors_search.html"), "careers.tatamotors.com")
    assert total == 144
    assert [j.source_job_id for j in jobs] == ["913208201", "913208001", "883136801"]
    j = jobs[0]
    assert j.title == "EV - Product Specialist"
    assert j.apply_url == "https://careers.tatamotors.com/job/Bangalore-EV-Product-Specialist/913208201/"
    assert j.cities == ["Bengaluru"]
    assert j.posted_at == datetime(2026, 10, 13, tzinfo=timezone.utc)
    assert j.partial and j.description is None


def test_search_page_lupin_state_and_country_codes():
    jobs, total = sf.parse_search(read("successfactors_lupin_search.html"), "careers.lupin.com")
    assert total == 90
    assert [j.cities for j in jobs] == [["Santacruz"], ["Airoli"], ["Santacruz"], ["Nagpur"]]  # "Nagpur, MH, IN"
    assert jobs[2].title == "Manager – Corporate Projects"


def test_job_page_adds_description_and_location():
    listed = sf.parse_search(read("successfactors_tatamotors_search.html"), "careers.tatamotors.com")[0][0]
    j = sf.parse_job(read("successfactors_tatamotors_job.html"), listed)
    assert j.title == "Senior Manager- Warehousing operations"
    assert j.cities == ["Sanand"]  # the page's own location wins
    assert j.description.startswith("<h2>Position Summary with Job Responsibilities</h2>\n<p>Purpose of the position")
    assert "<p><p>" not in j.description
    assert 'style="' not in j.description
    assert not j.partial and j.source_job_id == listed.source_job_id


def test_job_page_without_description_raises():
    listed = sf.parse_search(read("successfactors_tatamotors_search.html"), "careers.tatamotors.com")[0][0]
    with pytest.raises(ParseError):
        sf.parse_job("<html><body><h1>Sorry</h1></body></html>", listed)


def test_javascript_only_site_raises():
    with pytest.raises(ParseError, match="JavaScript"):
        sf.parse_search("<html><body><div id='app'></div></body></html>", "careers.wipro.com")


def test_empty_results_table_is_no_jobs():
    assert sf.parse_search("<table id='searchresults'><tbody></tbody></table>", "x.com") == ([], 0)


@pytest.mark.parametrize("bad", ["", "https://careers.x.com", "x.com/search"])
def test_bad_slug(bad):
    with pytest.raises(ParseError):
        sf.search_url(bad, 0)


# --- fetch() against a fake site -----------------------------------------------------------

class FakeSF:
    def __init__(self, n=60, page=25, gone=()):
        self.ids = [str(1000 + i) for i in range(n)]
        self.page, self.gone = page, set(gone)
        self.searches: list[str] = []
        self.details: list[str] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path == "/robots.txt":
            return httpx.Response(200, text="User-agent: *\nDisallow: /services/\n")
        if path == "/search/":
            assert request.url.params["locationsearch"] == "India"
            start = int(request.url.params["startrow"])
            self.searches.append(start)
            ids = self.ids[start:start + self.page]
            rows = "".join(
                f'<tr class="data-row"><td class="colTitle"><a href="/job/Pune-Role-{i}/{i}/" class="jobTitle-link">'
                f'Role {i}</a><span class="jobDate">Oct 1, 2026</span></td>'
                f'<td class="colLocation"><span class="jobLocation">Pune, MH, IN</span></td></tr>' for i in ids)
            label = f'<span class="paginationLabel">Results <b>{start + 1} – {start + len(ids)}</b> of <b>{len(self.ids)}</b></span>'
            return httpx.Response(200, text=f"<html><body>{label}<table id='searchresults'>{rows}</table></body></html>")
        if path.startswith("/job/"):
            job_id = path.rstrip("/").rsplit("/", 1)[-1]
            self.details.append(job_id)
            if job_id in self.gone:
                return httpx.Response(404)
            return httpx.Response(200, text=f'<span itemprop="title">Role {job_id}</span>'
                                            f'<span class="jobGeoLocation">Pune, India</span>'
                                            f'<span itemprop="description" class="jobdescription"><p>Do {job_id}</p></span>')
        return httpx.Response(404)


def client(server):
    return PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                        transport=httpx.MockTransport(server))


@pytest.mark.anyio
async def test_fetch_pages_and_details_only_new_jobs():
    server = FakeSF(n=60)
    async with client(server) as c:
        jobs = await sf.fetch(c, "careers.acme.com", {"1000": None, "1001": None})
    assert server.searches == [0, 25, 50]
    assert len(jobs) == 60 and len(server.details) == 58
    by_id = {j.source_job_id: j for j in jobs}
    assert by_id["1000"].partial and by_id["1000"].cities == ["Pune"]
    assert by_id["1002"].description == "<p>Do 1002</p>" and not by_id["1002"].partial


@pytest.mark.anyio
async def test_fetch_cap_and_gone_jobs(monkeypatch):
    monkeypatch.setattr(sf, "MAX_NEW_DETAILS", 5)
    server = FakeSF(n=10, gone={"1001"})
    async with client(server) as c:
        jobs = await sf.fetch(c, "careers.acme.com", {})
    assert len(server.details) == 5
    assert [j.source_job_id for j in jobs] == ["1000", "1002", "1003", "1004"]


@pytest.mark.anyio
async def test_detail_server_error_fails_the_company():
    server = FakeSF(n=3)

    def broken(request):
        return httpx.Response(500) if request.url.path.startswith("/job/") else server(request)

    async with client(broken) as c:
        with pytest.raises(FetchError):
            await sf.fetch(c, "careers.acme.com", {})


@pytest.mark.anyio
async def test_too_many_pages_fails_instead_of_closing(monkeypatch):
    monkeypatch.setattr(sf, "MAX_PAGES", 2)
    async with client(FakeSF(n=80)) as c:
        with pytest.raises(ParseError, match="cut off"):
            await sf.fetch(c, "careers.acme.com", {})


@pytest.mark.parametrize("text, want", [
    ("Oct 13, 2026", datetime(2026, 10, 13, tzinfo=timezone.utc)),
    ("28 Sept 2026", datetime(2026, 9, 28, tzinfo=timezone.utc)),  # Tata Consumer
    ("5 Oct 2026", datetime(2026, 10, 5, tzinfo=timezone.utc)),
    ("", None),
    ("yesterday", None),
])
def test_dates(text, want):
    assert sf._date(text) == want
