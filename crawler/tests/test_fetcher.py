import asyncio
import time

import httpx
import pytest

from fetcher import FetchError, PoliteClient, RobotsDisallowed

UA = "CareerGlobeBot/0.1 (+https://github.com/Vedd-Patel/CareerGlobe)"
pytestmark = pytest.mark.anyio


class Server:
    """Fake web server: records every request, answers from a handler."""

    def __init__(self, handler, robots="User-agent: *\nAllow: /\n"):
        self.handler, self.robots = handler, robots
        self.requests: list[tuple[float, httpx.Request]] = []

    def __call__(self, request: httpx.Request) -> httpx.Response:
        self.requests.append((time.monotonic(), request))
        if request.url.path == "/robots.txt":
            if isinstance(self.robots, int):
                return httpx.Response(self.robots)
            return httpx.Response(200, text=self.robots)
        return self.handler(request)

    def paths(self):
        return [f"{r.url.host}{r.url.path}" for _, r in self.requests]


def client(server, min_gap=0.0, max_gap=0.0, **kw):
    return PoliteClient(
        UA, "CareerGlobeBot", min_gap=min_gap, max_gap=max_gap, backoff=0,
        transport=httpx.MockTransport(server), **kw,
    )


async def test_sends_user_agent_and_checks_robots_first():
    server = Server(lambda r: httpx.Response(200, json=[]))
    async with client(server) as c:
        await c.get("https://api.lever.co/v0/postings/cred?mode=json")
    assert server.paths() == ["api.lever.co/robots.txt", "api.lever.co/v0/postings/cred"]
    assert all(r.headers["User-Agent"] == UA for _, r in server.requests)


async def test_robots_fetched_once_per_host():
    server = Server(lambda r: httpx.Response(200, json=[]))
    async with client(server) as c:
        await c.get("https://h.com/a")
        await c.get("https://h.com/b")
    assert server.paths().count("h.com/robots.txt") == 1


async def test_robots_disallow_raises_without_fetching():
    server = Server(lambda r: httpx.Response(200), robots="User-agent: *\nDisallow: /\n")
    async with client(server) as c:
        with pytest.raises(RobotsDisallowed):
            await c.get("https://h.com/jobs")
    assert server.paths() == ["h.com/robots.txt"]


async def test_missing_robots_allows_but_forbidden_robots_blocks():
    async with client(Server(lambda r: httpx.Response(200), robots=404)) as c:
        await c.get("https://h.com/jobs")
    async with client(Server(lambda r: httpx.Response(200), robots=403)) as c:
        with pytest.raises(RobotsDisallowed):
            await c.get("https://h.com/jobs")


async def test_robots_server_error_fails_the_fetch():
    server = Server(lambda r: httpx.Response(200), robots=503)
    async with client(server, max_attempts=2) as c:
        with pytest.raises(FetchError) as e:
            await c.get("https://h.com/jobs")
    assert not isinstance(e.value, RobotsDisallowed)
    assert server.paths() == ["h.com/robots.txt"] * 2


async def test_post_json_obeys_robots_and_sends_json():
    import json

    seen = []

    def handler(request):
        seen.append((request.method, request.headers["Content-Type"], json.loads(request.content)))
        return httpx.Response(200, json={"jobPostings": []})

    server = Server(handler, robots="User-agent: *\nDisallow: /refreshFacet/\n")
    async with client(server) as c:
        res = await c.post_json("https://t.wd5.myworkdayjobs.com/wday/cxs/t/s/jobs", {"offset": 0})
        with pytest.raises(RobotsDisallowed):
            await c.post_json("https://t.wd5.myworkdayjobs.com/refreshFacet/x", {})
    assert res.json() == {"jobPostings": []}
    assert seen == [("POST", "application/json", {"offset": 0})]
    assert server.paths()[0] == "t.wd5.myworkdayjobs.com/robots.txt"


async def test_etag_round_trip_and_304():
    def handler(request):
        if request.headers.get("If-None-Match") == 'W/"v1"':
            return httpx.Response(304)
        return httpx.Response(200, json={"jobs": []}, headers={"ETag": 'W/"v1"'})

    async with client(Server(handler)) as c:
        first = await c.get("https://h.com/jobs")
        second = await c.get("https://h.com/jobs", etag=first.etag)
    assert first.status == 200 and first.etag == 'W/"v1"' and first.json() == {"jobs": []}
    assert second.not_modified and second.etag == 'W/"v1"'


async def test_sends_if_modified_since():
    seen = {}

    def handler(request):
        seen.update(request.headers)
        return httpx.Response(304)

    async with client(Server(handler)) as c:
        await c.get("https://h.com/jobs", last_modified="Mon, 19 Oct 2026 10:00:00 GMT")
    assert seen["if-modified-since"] == "Mon, 19 Oct 2026 10:00:00 GMT"


async def test_retries_5xx_then_succeeds():
    calls = iter([503, 502, 200])
    server = Server(lambda r: httpx.Response(next(calls)))
    async with client(server, max_attempts=3) as c:
        result = await c.get("https://h.com/jobs")
    assert result.status == 200
    assert server.paths().count("h.com/jobs") == 3


async def test_gives_up_after_max_attempts():
    server = Server(lambda r: httpx.Response(429))
    async with client(server, max_attempts=3) as c:
        with pytest.raises(FetchError) as e:
            await c.get("https://h.com/jobs")
    assert e.value.status == 429
    assert server.paths().count("h.com/jobs") == 3


async def test_does_not_retry_404():
    server = Server(lambda r: httpx.Response(404))
    async with client(server) as c:
        with pytest.raises(FetchError) as e:
            await c.get("https://h.com/jobs")
    assert e.value.status == 404
    assert server.paths().count("h.com/jobs") == 1


async def test_network_error_becomes_fetch_error():
    def handler(request):
        raise httpx.ConnectError("refused")

    async with client(Server(handler), max_attempts=2) as c:
        with pytest.raises(FetchError):
            await c.get("https://h.com/jobs")


async def test_same_host_is_serialised_with_gap():
    server = Server(lambda r: httpx.Response(200))
    async with client(server, min_gap=0.1, max_gap=0.12) as c:
        await asyncio.gather(*(c.get(f"https://h.com/{i}") for i in range(3)))
    times = [t for t, _ in server.requests]  # robots + 3 pages
    gaps = [b - a for a, b in zip(times, times[1:])]
    assert len(gaps) == 3 and min(gaps) >= 0.095


async def test_different_hosts_run_in_parallel():
    server = Server(lambda r: httpx.Response(200))
    start = time.monotonic()
    async with client(server, min_gap=0.2, max_gap=0.2) as c:
        await asyncio.gather(*(c.get(f"https://h{i}.com/jobs") for i in range(5)))
    # Each host: robots, gap, page = ~0.2 s. Serial would be ~2 s.
    assert time.monotonic() - start < 0.6


async def test_crawl_delay_longer_than_gap_is_honoured():
    server = Server(lambda r: httpx.Response(200), robots="User-agent: *\nCrawl-delay: 0.3\n")
    async with client(server, min_gap=0.0, max_gap=0.0) as c:
        await c.get("https://h.com/a")
        await c.get("https://h.com/b")
    times = [t for t, _ in server.requests]
    assert times[2] - times[1] >= 0.29
