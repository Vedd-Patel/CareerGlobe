"""JSON-LD + sitemap reader for custom careers pages (hand-made fixtures:
no Indian careers site with JobPosting JSON-LD was found on 2026-10-21)."""

import gzip
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ParseError, jsonld
from fetcher import PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"
CAREERS = "https://acme.example/careers"


def read(name):
    return (FIXTURES / name).read_text()


def test_job_page_posting():
    found = jsonld.postings(read("jsonld_job_page.html"))
    assert len(found) == 1  # the Organization in @graph is ignored
    j = jsonld.to_job(found[0], "https://acme.example/careers/jobs/backend-engineer-123")
    assert j.title == "Backend Engineer"
    assert j.apply_url == "https://acme.example/careers/jobs/backend-engineer-123?utm_source=google"
    assert j.source_job_id == "https://acme.example/careers/jobs/backend-engineer-123"  # canonical
    assert j.cities == ["Bengaluru", "Gurugram"]
    assert (j.salary_min, j.salary_max) == (1_800_000, 2_400_000)  # INR per month -> per year
    assert j.description == "<p>Build <b>APIs</b></p>"  # unescaped, script dropped
    assert j.posted_at == datetime(2026, 10, 1, tzinfo=timezone.utc)


def test_listing_page_item_list():
    found = jsonld.postings(read("jsonld_listing_page.html"))  # the broken script is skipped
    assert [p["title"] for p in found] == ["Data Analyst", "Store Manager, Austin", "Old Role"]
    now = datetime(2026, 10, 21, tzinfo=timezone.utc)
    jobs = [jsonld.to_job(p, CAREERS, now) for p in found]
    assert jobs[0].work_mode == "remote" and jobs[0].cities == []  # remote, India only
    assert jobs[1] is None  # US only
    assert jobs[2] is None  # expired


def test_posting_without_title_is_skipped():
    assert jsonld.to_job({"@type": "JobPosting", "url": "/x"}, CAREERS) is None


def test_sitemap_index_and_urlset():
    pages, children = jsonld.parse_sitemap(read("jsonld_sitemap_index.xml").encode())
    assert pages == [] and children[1] == "https://acme.example/sitemap-jobs.xml"
    pages, children = jsonld.parse_sitemap(gzip.compress(read("jsonld_sitemap_jobs.xml").encode()))
    assert children == []
    assert ("https://www.acme.example/careers/jobs/sre-5?team=a&lang=en", "2026-10-02") in pages


@pytest.mark.parametrize("url, want", [
    ("https://acme.example/careers/jobs/backend-engineer-123", True),
    ("https://www.acme.example/careers/jobs/sre-5", True),  # www. is the same site
    ("https://acme.example/careers", False),  # the careers page itself
    ("https://acme.example/careers/", False),
    ("https://acme.example/about/team", False),
    ("https://acme.example/jobs/42", True),  # outside the careers path but job-like
    ("https://acme.example/careers/jobs/brochure.pdf", False),
    ("https://other.example/careers/jobs/1", False),
])
def test_job_like(url, want):
    assert jsonld.job_like(url, CAREERS) is want


# --- fetch() against a fake site -----------------------------------------------------------

class FakeSite:
    def __init__(self):
        self.listing = "<html><body>No jobs here</body></html>"
        self.sitemap = read("jsonld_sitemap_jobs.xml")
        self.robots = "User-agent: *\nAllow: /\nSitemap: https://acme.example/sitemap_index.xml\n"
        self.job_pages = {
            "/careers/jobs/backend-engineer-123": read("jsonld_job_page.html"),
            "/careers/jobs/sre-5": read("jsonld_job_page.html").replace("Backend Engineer", "SRE")
                                                         .replace("/careers/jobs/backend-engineer-123", "/careers/jobs/sre-5"),
        }
        self.fetched: list[str] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        path = request.url.path
        if path == "/robots.txt":
            return httpx.Response(200, text=self.robots)
        self.fetched.append(f"{request.url.host}{path}")
        if path == "/careers":
            return httpx.Response(200, text=self.listing)
        if path == "/sitemap_index.xml":
            return httpx.Response(200, text=read("jsonld_sitemap_index.xml"))
        if path == "/sitemap-jobs.xml":
            return httpx.Response(200, text=self.sitemap)
        if path in self.job_pages:
            return httpx.Response(200, text=self.job_pages[path])
        return httpx.Response(404)


def client(server):
    return PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0,
                        transport=httpx.MockTransport(server))


@pytest.mark.anyio
async def test_fetch_reads_job_pages_from_the_job_sitemap():
    site = FakeSite()
    async with client(site) as c:
        jobs = await jsonld.fetch(c, CAREERS, {})
    assert sorted(j.title for j in jobs) == ["Backend Engineer", "SRE"]
    assert "acme.example/sitemap-pages.xml" not in site.fetched  # the index's job sitemap is preferred
    assert not any("other.example" in f for f in site.fetched)  # other hosts are never read
    sre = next(j for j in jobs if j.title == "SRE")
    assert sre.source_updated == "2026-10-02" and sre.apply_url.startswith("https://www.acme.example/")


@pytest.mark.anyio
async def test_unchanged_pages_are_not_fetched_again():
    site = FakeSite()
    backend = "https://acme.example/careers/jobs/backend-engineer-123"
    sre = "https://www.acme.example/careers/jobs/sre-5?lang=en&team=a"
    async with client(site) as c:
        jobs = await jsonld.fetch(c, CAREERS, {backend: "2026-10-01", sre: "2026-09-01"})
    assert [f for f in site.fetched if "/jobs/" in f] == ["www.acme.example/careers/jobs/sre-5"]  # lastmod changed
    by_id = {j.source_job_id: j for j in jobs}
    assert by_id[backend].partial and by_id[backend].title == ""
    assert not by_id[sre].partial and by_id[sre].source_updated == "2026-10-02"


@pytest.mark.anyio
async def test_changed_known_page_over_the_cap_stays_open(monkeypatch):
    monkeypatch.setattr(jsonld, "MAX_NEW_DETAILS", 0)
    site = FakeSite()
    backend = "https://acme.example/careers/jobs/backend-engineer-123"
    async with client(site) as c:
        jobs = await jsonld.fetch(c, CAREERS, {backend: "2025-01-01"})
    assert [j.source_job_id for j in jobs] == [backend] and jobs[0].partial


@pytest.mark.anyio
async def test_listing_page_postings_need_no_sitemap():
    site = FakeSite()
    site.listing = read("jsonld_listing_page.html")
    site.robots = "User-agent: *\nAllow: /\n"  # no Sitemap: line and /sitemap.xml is 404
    async with client(site) as c:
        jobs = await jsonld.fetch(c, CAREERS, {})
    assert [j.title for j in jobs] == ["Data Analyst"]
    assert "acme.example/sitemap.xml" in site.fetched


@pytest.mark.anyio
async def test_nothing_found_fails_instead_of_closing():
    site = FakeSite()
    site.robots = "User-agent: *\nAllow: /\n"
    async with client(site) as c:
        with pytest.raises(ParseError, match="no JobPosting"):
            await jsonld.fetch(c, CAREERS, {})


@pytest.mark.anyio
async def test_job_pages_without_json_ld_fail_on_first_crawl():
    site = FakeSite()
    site.job_pages = {k: "<html><body>JS app</body></html>" for k in site.job_pages}
    async with client(site) as c:
        with pytest.raises(ParseError, match="none of 2 sitemap pages"):
            await jsonld.fetch(c, CAREERS, {})
