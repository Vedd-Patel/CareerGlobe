"""Logos from a company's home page: which icon is picked, and that only real images are kept."""

import httpx
import pytest

import logos
from fetcher import PoliteClient

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
                    return [(7, "https://acme.in/")]
            return Cur()

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, max_attempts=1, transport=httpx.MockTransport(handler)) as c:
        assert await logos.run(Conn(), c) == (1, 1)
    assert updates == [("https://acme.in/touch-120.png", 7)]
