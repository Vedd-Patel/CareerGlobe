"""Selector-based reader for custom careers pages."""

import httpx
import pytest

from adapters import ParseError, html, source_for
from fetcher import PoliteClient

PAGE1 = """<html><body>
<div class="opening"><a href="/careers/42?utm_source=x"><h3>Backend Engineer</h3></a><span class="loc">Pune, India</span></div>
<div class="opening"><a href="https://acme.example/careers/43"><h3>Designer</h3></a><span class="loc">Remote</span></div>
<div class="opening"><span>No link here</span></div>
<a class="next" href="/careers?page=2">Next</a>
</body></html>"""
PAGE2 = """<div class="opening"><a href="/careers/44"><h3>QA Engineer</h3></a><span class="loc">Bangalore</span></div>"""
SEL = {"item": "div.opening", "title": "h3", "link": "a", "location": ".loc", "next": "a.next"}


def test_parse_page():
    jobs, nxt = html.parse_page(PAGE1, "https://acme.example/careers", SEL)
    assert [(j.title, j.cities, j.work_mode) for j in jobs] == [
        ("Backend Engineer", ["Pune"], None), ("Designer", [], "remote")]
    assert jobs[0].source_job_id == "https://acme.example/careers/42"  # tracking params stripped
    assert nxt == "https://acme.example/careers?page=2"


def test_needs_an_item_selector():
    with pytest.raises(ParseError):
        html.parse_page(PAGE1, "https://acme.example/careers", {"title": "h3"})


def test_source_for_html():
    assert source_for("https://acme.example/careers", "html") == ("html", "https://acme.example/careers")


@pytest.mark.anyio
async def test_fetch_follows_next_pages_and_fails_when_selectors_find_nothing():
    pages = {"/careers": PAGE1, "/careers?page=2": PAGE2}

    def handler(request):
        if request.url.path == "/robots.txt":
            return httpx.Response(404)
        key = request.url.path + (f"?{request.url.query.decode()}" if request.url.query else "")
        return httpx.Response(200, text=pages.get(key, "<p>nothing</p>"))

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, transport=httpx.MockTransport(handler)) as c:
        jobs = await html.fetch(c, "https://acme.example/careers", {}, selectors=SEL)
        assert [j.title for j in jobs] == ["Backend Engineer", "Designer", "QA Engineer"]
        with pytest.raises(ParseError, match="found no jobs"):
            await html.fetch(c, "https://acme.example/other", {}, selectors=SEL)
