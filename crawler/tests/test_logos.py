"""Logos from a company's home page: which icon is picked, and that only real images are kept."""

import httpx
import pytest

import logos
from fetcher import PoliteClient
from fetcher import FetchError  # noqa: F401

PAGE = """<html><head>
<link rel="icon" href="/favicon.ico" sizes="16x16">
<link rel="icon" type="image/svg+xml" href="/logo.svg">
<link rel="apple-touch-icon" sizes="120x120" href="/touch-120.png">
<link rel="apple-touch-icon" sizes="180x180" href="https://cdn.acme.in/touch-180.png">
<link rel="icon" href="http://plain.example/insecure.png">
<link rel="icon" href="https://localhost/internal.png">
<link rel="icon" href="data:image/png;base64,AAAA">
</head></html>"""
PNG = b"\x89PNG\r\n\x1a\n" + b"0" * 40


def test_touch_icons_first_largest_first_and_unsafe_ones_dropped():
    got = logos.candidates(PAGE, "https://acme.in/")
    assert got == ["https://cdn.acme.in/touch-180.png", "https://acme.in/touch-120.png",
                   "https://acme.in/logo.svg", "https://acme.in/favicon.ico"]


def test_looks_like_image():
    assert logos.looks_like_image(PNG) and logos.looks_like_image(b'  <?xml version="1.0"?><svg xmlns="x"/>')
    assert not logos.looks_like_image(b"<html>not found</html>") and not logos.looks_like_image(b"")
    assert not logos.looks_like_image(PNG + b"0" * logos.MAX_BYTES)


@pytest.mark.anyio
async def test_run_keeps_the_first_candidate_that_is_an_image():
    def handler(request):
        p = request.url.path
        if p == "/robots.txt":
            return httpx.Response(404)
        if p == "/":
            return httpx.Response(200, text=PAGE)
        if p == "/touch-120.png":
            return httpx.Response(200, content=PNG)
        return httpx.Response(404)  # the 180px icon on the CDN is gone

    updates = []

    class Conn:
        async def execute(self, sql, params=None):
            if sql.lstrip().startswith("UPDATE"):
                updates.append(params)

                class Done: ...
                return Done()

            class Cur:
                async def fetchall(self):
                    return [(7, "https://acme.in/", None)]
            return Cur()

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, max_attempts=1, transport=httpx.MockTransport(handler)) as c:
        assert await logos.run(Conn(), c) == (1, 1)
    assert updates == [("https://acme.in/touch-120.png", 7)]


KEKA = """<html><body><img class="org-logo w-50" alt="Keka Hire" src="/ats/documents/abc/orglogo/logo123.png">
<img width="50" src="https://cdn.keka.com/careers/Keka_Logo_Black_Text.svg"></body></html>"""
LEVER = """<meta property="og:image" content="https://lever-client-logos.s3.us-west-2.amazonaws.com/x-1.png" />"""
LEVER_OTHER = """<meta property="og:image" content="https://evil.example/banner.png" />"""
ASHBY = """<link rel="icon" href="https://cdn.ashbyprd.com/favicon.svg"><meta property="og:image" content="https://app.ashbyhq.com/api/images/org-theme-logo/a/b/c.png" />"""


def test_board_logos_come_from_the_board_not_its_vendor_icons():
    assert logos.board_logo(KEKA, "https://acme.keka.com/careers/") == "https://acme.keka.com/ats/documents/abc/orglogo/logo123.png"
    assert logos.board_logo(LEVER, "https://jobs.lever.co/acme").endswith("x-1.png")
    assert logos.board_logo(LEVER_OTHER, "https://jobs.lever.co/acme") is None
    assert logos.board_logo(ASHBY, "https://jobs.ashbyhq.com/acme").endswith("/org-theme-logo/a/b/c.png")
    assert logos.board_logo("<html></html>", "https://acme.keka.com/careers/") is None
    assert logos.board_logo(KEKA, "https://careers.acme.in/") is None  # only vendor boards use these rules


def test_vendor_hosts_and_standard_icon_places():
    assert logos.is_vendor("acme.keka.com") and logos.is_vendor("nvidia.wd5.myworkdayjobs.com") and not logos.is_vendor("acme.in")
    assert logos.standard_icons("https://acme.in/jobs/x") == ["https://acme.in/apple-touch-icon.png", "https://acme.in/favicon.ico"]
