"""Ashby board and job pages against saved real pages (Sarvam, trimmed, 2026-11-12)."""

import json
from datetime import datetime, timezone
from pathlib import Path

import httpx
import pytest

from adapters import ADAPTERS, ParseError, ashby, detect
from fetcher import PoliteClient

FIXTURES = Path(__file__).parent / "fixtures"
LIST = (FIXTURES / "ashby_sarvam_list.html").read_text()
JOB = (FIXTURES / "ashby_sarvam_job.html").read_text()
IDS = ["36f89b00-2010-4d23-aae3-17a2f53d9eaa", "7c42207a-bb56-413c-839f-9ad692df840e",
       "dc047f4f-97be-4bb3-a9f1-cebbfc4de3e6"]


def test_list_has_every_job_without_descriptions():
    jobs = ashby.parse_list(LIST, "sarvam")
    assert [j.source_job_id for j in jobs] == IDS
    j = jobs[0]
    assert j.title == "Agent Engineer"
    assert j.apply_url == f"https://jobs.ashbyhq.com/sarvam/{IDS[0]}"
    assert j.cities == ["Bengaluru"] and j.in_india is True
    assert j.work_mode == "onsite"
    assert j.partial and j.description is None
    assert jobs[2].cities == ["Delhi"]


def test_detail_has_description_and_posted_date():
    j = ashby.parse_detail(JOB, "sarvam")
    assert (j.source_job_id, j.title, j.cities) == (IDS[0], "Agent Engineer", ["Bengaluru"])
    assert j.description.startswith("<p>The distance between an AI system that works")
    assert "style=" not in j.description
    assert j.posted_at == datetime(2026, 8, 20, tzinfo=timezone.utc)
    assert not j.partial


def page(postings):
    return "<script>window.__appData = " + json.dumps({"jobBoard": {"jobPostings": postings}}) + ";</script>"


def test_foreign_and_unclear_locations():
    jobs = ashby.parse_list(page([
        {"id": "a", "title": "SRE", "locationName": "San Francisco", "workplaceType": "Hybrid", "secondaryLocations": []},
        {"id": "b", "title": "PM", "locationName": "Remote", "workplaceType": "Remote", "secondaryLocations": []},
        {"id": "c", "title": "Eng", "locationName": "New York",
         "secondaryLocations": [{"locationName": "Bangalore"}]},
        {"id": "", "title": "no id"},
    ]), "acme")
    assert [(j.source_job_id, j.in_india, j.work_mode) for j in jobs] == [
        ("a", False, "hybrid"), ("b", None, "remote"), ("c", True, None)]
    assert "Bengaluru" in jobs[2].cities


@pytest.mark.parametrize("bad", ["<html>no data</html>", "<script>window.__appData = {oops</script>",
                                 "<script>window.__appData = {\"jobBoard\": null};</script>"])
def test_unexpected_page_raises(bad):
    with pytest.raises(ParseError):
        ashby.parse_list(bad, "x")
    with pytest.raises(ParseError):
        ashby.parse_detail(bad, "x")


def test_detect():
    assert detect("https://jobs.ashbyhq.com/sarvam") == ("ashby", "sarvam")
    assert detect(f"https://jobs.ashbyhq.com/Sarvam/{IDS[0]}") == ("ashby", "Sarvam")
    assert detect("https://jobs.ashbyhq.com/api/non-user-graphql") is None


@pytest.mark.anyio
async def test_fetch_reads_details_only_for_new_india_jobs():
    listing = page([
        {"id": IDS[0], "title": "Agent Engineer", "locationName": "Bengaluru", "secondaryLocations": []},
        {"id": "known", "title": "Known job", "locationName": "Pune", "secondaryLocations": []},
        {"id": "abroad", "title": "Abroad", "locationName": "London", "secondaryLocations": []},
        {"id": "gone", "title": "Closed meanwhile", "locationName": "Delhi", "secondaryLocations": []},
    ])
    seen = []

    def handler(request):
        seen.append(request.url.path)
        if request.url.path == "/robots.txt":
            return httpx.Response(200, text="User-Agent: *\nDisallow: /api/\n")
        if request.url.path == "/sarvam":
            return httpx.Response(200, text=listing)
        if request.url.path == f"/sarvam/{IDS[0]}":
            return httpx.Response(200, text=JOB)
        return httpx.Response(404)

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, max_attempts=1, transport=httpx.MockTransport(handler)) as c:
        jobs = await ADAPTERS["ashby"].fetch(c, "sarvam", {"known": None})
    assert [(j.source_job_id, j.partial) for j in jobs] == [(IDS[0], False), ("known", True)]
    assert seen == ["/robots.txt", "/sarvam", f"/sarvam/{IDS[0]}", "/sarvam/gone"]  # never /sarvam/abroad
