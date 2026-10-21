import httpx
import pytest

import crawl
from heartbeat import Heartbeat

pytestmark = pytest.mark.anyio


async def test_disabled_without_url(monkeypatch):
    monkeypatch.delenv("HEALTHCHECK_CRAWL_URL", raising=False)
    hb = Heartbeat("HEALTHCHECK_CRAWL_URL", "Bot/1")
    assert not hb.enabled
    await hb.start()  # no network, no error


async def test_pings_start_success_and_fail(monkeypatch):
    seen = []

    def handler(request):
        seen.append((request.url.path, request.content.decode()))
        return httpx.Response(200)

    real = httpx.AsyncClient
    monkeypatch.setattr(httpx, "AsyncClient", lambda **kw: real(transport=httpx.MockTransport(handler), **kw))
    monkeypatch.setenv("HEALTHCHECK_CRAWL_URL", "https://hc-ping.com/abc/")
    hb = Heartbeat("HEALTHCHECK_CRAWL_URL", "Bot/1")
    await hb.start()
    await hb.success("all good")
    await hb.fail("x" * 20_000)
    assert [p for p, _ in seen] == ["/abc/start", "/abc", "/abc/fail"]
    assert seen[1][1] == "all good" and len(seen[2][1]) == 10_000


async def test_ping_errors_are_swallowed(monkeypatch):
    def handler(request):
        raise httpx.ConnectError("down")

    real = httpx.AsyncClient
    monkeypatch.setattr(httpx, "AsyncClient", lambda **kw: real(transport=httpx.MockTransport(handler), **kw))
    monkeypatch.setenv("HEALTHCHECK_CRAWL_URL", "https://hc-ping.com/abc")
    await Heartbeat("HEALTHCHECK_CRAWL_URL", "Bot/1").success()


@pytest.mark.parametrize("ok, failed, not_started, bad", [
    (90, 10, 0, False), (89, 11, 0, True), (100, 0, 3, True), (0, 0, 0, False)])
def test_bad_night(ok, failed, not_started, bad):
    assert crawl.bad_night(crawl.Tally(ok=ok, failed=failed, not_started=not_started)) is bad
