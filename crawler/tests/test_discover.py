"""discover.py: finding careers pages and job boards from websites (fake sites)."""

import httpx
import pytest

import discover
from fetcher import PoliteClient


def test_read_websites(tmp_path):
    f = tmp_path / "new.txt"
    f.write_text("# comment\nAcme Corp,https://www.acme.example/\nbeta-labs.example\n\n")
    assert discover.read_websites(f) == [("Acme Corp", "https://www.acme.example"),
                                         ("Beta Labs", "https://beta-labs.example")]


def test_candidates():
    assert discover.candidates("https://www.acme.example") == [
        "https://www.acme.example/careers", "https://www.acme.example/jobs",
        "https://careers.acme.example", "https://jobs.acme.example"]


@pytest.mark.parametrize("url, html, want", [
    ("https://jobs.lever.co/acme", "", ("https://jobs.lever.co/acme", "lever")),
    ("https://acme.example/careers", '<a href="https://boards.greenhouse.io/acme/jobs/123">Job</a>',
     ("https://boards.greenhouse.io/acme", "greenhouse")),
    ("https://acme.example/careers", '<iframe src="https://acme.keka.com/careers/jobdetails/5"></iframe>',
     ("https://acme.keka.com/careers/", "keka")),
    ("https://acme.example/careers",
     '<a href="https://acme.wd3.myworkdayjobs.com/en-US/AcmeCareers/job/Pune/X_R1">x</a>',
     ("https://acme.wd3.myworkdayjobs.com/AcmeCareers", "workday")),
    ("https://acme.example/careers", '<a href="https://jobs.ashbyhq.com/acme/123">x</a>',
     ("https://jobs.ashbyhq.com/acme", "ashby")),
    ("https://careers.acme.example/", '<table><tr class="data-row"><td><a class="jobTitle-link" href="/job/x/1/">x</a></td></tr></table>',
     ("https://careers.acme.example/", "successfactors")),
    ("https://acme.example/careers", '<script type="application/ld+json">{"@type": "JobPosting", "title": "x"}</script>',
     ("https://acme.example/careers", "")),
    ("https://acme.example/careers", "<title>Careers at Acme</title>", ("https://acme.example/careers", "")),
])
def test_classify(url, html, want):
    found = discover.classify(url, html)
    assert (found.careers_url, found.ats) == want


def test_classify_rejects_a_page_that_is_not_about_jobs():
    assert discover.classify("https://acme.example/", "<title>Acme - Home</title>") is None


class FakeWeb:
    def __init__(self, pages):
        self.pages, self.hits = pages, []

    def __call__(self, request):
        if request.url.path == "/robots.txt":
            return httpx.Response(404)
        key = f"{request.url.host}{request.url.path}"
        self.hits.append(key)
        if key in self.pages:
            return httpx.Response(200, text=self.pages[key])
        return httpx.Response(404)


def client(web):
    return PoliteClient("CareerGlobeBot/test", "CareerGlobeBot", 0, 0, backoff=0, max_attempts=1,
                        transport=httpx.MockTransport(web))


@pytest.mark.anyio
async def test_discover_stops_at_the_first_careers_page():
    web = FakeWeb({"acme.example/jobs": '<a href="https://jobs.lever.co/acme/1">Apply</a>'})
    async with client(web) as c:
        found = await discover.discover_one(c, "Acme", "https://acme.example")
    assert (found.careers_url, found.ats) == ("https://jobs.lever.co/acme", "lever")
    assert "careers.acme.example/" not in web.hits  # stopped after /jobs


@pytest.mark.anyio
async def test_discover_falls_back_to_the_homepage_link():
    web = FakeWeb({"acme.example/": '<a href="/about">About</a><a href="/work-with-us">Join us</a>',
                   "acme.example/work-with-us": '<a href="https://acme.keka.com/careers/">Openings</a>'})
    async with client(web) as c:
        found = await discover.discover_one(c, "Acme", "https://acme.example")
    assert (found.careers_url, found.ats) == ("https://acme.keka.com/careers/", "keka")
    assert "via homepage" in found.note


@pytest.mark.anyio
async def test_discover_reports_not_found():
    async with client(FakeWeb({"acme.example/": "<p>hello</p>"})) as c:
        found = await discover.discover_one(c, "Acme", "https://acme.example")
    assert found.careers_url == "" and found.note.startswith("not found")


@pytest.mark.anyio
async def test_main_writes_seed_format(tmp_path, monkeypatch):
    web = FakeWeb({"acme.example/careers": '<a href="https://jobs.lever.co/acme">Jobs</a>'})
    monkeypatch.setattr(discover, "PoliteClient",
                        lambda *a, **k: PoliteClient(*a, **{**k, "transport": httpx.MockTransport(web),
                                                            "max_attempts": 1, "backoff": 0}))
    sites, out = tmp_path / "new.txt", tmp_path / "out.csv"
    sites.write_text("Acme,https://acme.example\n")
    monkeypatch.setenv("CRAWLER_MIN_GAP_SECONDS", "0")
    monkeypatch.setenv("CRAWLER_MAX_GAP_SECONDS", "0")
    assert await discover.main(["--websites", str(sites), "--out", str(out)]) == 0
    lines = out.read_text().splitlines()
    assert lines[0] == "name,website,careers_url,city,state,type,ats,note"
    assert lines[1].startswith("Acme,https://acme.example,https://jobs.lever.co/acme,,,,lever,discovered:")
