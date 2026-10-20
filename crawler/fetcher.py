"""Polite HTTP client.

- Honest user agent on every request.
- robots.txt checked per host before any other request (RFC 9309 rules).
- One request at a time per host, with a random 1-2 s gap (or the host's
  Crawl-delay if longer). Many companies share one ATS host, so this is per
  host, not per company.
- ETag / Last-Modified sent back, so unchanged boards answer 304.
- POST only for JSON search APIs that are read-only (Workday's job list).
- Retries only on network errors, 429 and 5xx.
"""

import asyncio
import logging
import random
import time
from dataclasses import dataclass, field
from urllib.parse import urlsplit

import httpx
from tenacity import AsyncRetrying, retry_if_exception_type, stop_after_attempt, wait_exponential

from robots import Robots

log = logging.getLogger(__name__)


class FetchError(Exception):
    """The page could not be fetched; the company counts as failed tonight."""

    def __init__(self, message: str, status: int | None = None):
        super().__init__(message)
        self.status = status


class RobotsDisallowed(FetchError):
    """robots.txt forbids this URL; skip and flag the company."""


class _Retryable(Exception):
    def __init__(self, status: int):
        super().__init__(f"HTTP {status}")
        self.status = status


@dataclass
class FetchResult:
    url: str
    status: int
    content: bytes
    etag: str | None
    last_modified: str | None

    @property
    def not_modified(self) -> bool:
        return self.status == 304

    def json(self):
        import json

        return json.loads(self.content)


@dataclass
class _Host:
    lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    robots_lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    next_allowed: float = 0.0
    robots: Robots | None = None


class PoliteClient:
    def __init__(
        self,
        user_agent: str,
        robots_token: str,
        min_gap: float = 1.0,
        max_gap: float = 2.0,
        timeout: float = 30.0,
        max_attempts: int = 3,
        backoff: float = 2.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ):
        self._token = robots_token
        self._min_gap, self._max_gap = min_gap, max_gap
        self._max_attempts, self._backoff = max_attempts, backoff
        self._hosts: dict[str, _Host] = {}
        self._client = httpx.AsyncClient(
            headers={"User-Agent": user_agent},
            timeout=timeout,
            follow_redirects=True,
            transport=transport,
        )

    async def __aenter__(self) -> "PoliteClient":
        return self

    async def __aexit__(self, *exc) -> None:
        await self._client.aclose()

    async def get(self, url: str, etag: str | None = None, last_modified: str | None = None) -> FetchResult:
        host = await self._allowed(url)
        headers = {}
        if etag:
            headers["If-None-Match"] = etag
        if last_modified:
            headers["If-Modified-Since"] = last_modified
        resp = await self._request(host, "GET", url, headers)
        return self._result(url, resp, etag, last_modified)

    async def post_json(self, url: str, payload: dict) -> FetchResult:
        """POST a JSON body to a read-only search API. Same robots.txt check,
        per-host lock, gap and retries as get(); no conditional headers."""
        host = await self._allowed(url)
        resp = await self._request(host, "POST", url, {"Accept": "application/json"}, payload)
        return self._result(url, resp, None, None)

    async def _allowed(self, url: str) -> _Host:
        parts = urlsplit(url)
        host = self._host(parts.netloc)
        robots = await self._robots(host, f"{parts.scheme}://{parts.netloc}")
        if not robots.can_fetch(self._token, url):
            raise RobotsDisallowed(f"robots.txt disallows {url}")
        return host

    @staticmethod
    def _result(url: str, resp: httpx.Response, etag: str | None, last_modified: str | None) -> FetchResult:
        if resp.status_code not in (200, 304):
            raise FetchError(f"HTTP {resp.status_code} for {url}", resp.status_code)
        return FetchResult(
            url=str(resp.url),
            status=resp.status_code,
            content=resp.content,
            etag=resp.headers.get("ETag") or (etag if resp.status_code == 304 else None),
            last_modified=resp.headers.get("Last-Modified") or (last_modified if resp.status_code == 304 else None),
        )

    def _host(self, netloc: str) -> _Host:
        return self._hosts.setdefault(netloc.lower(), _Host())

    async def _robots(self, host: _Host, origin: str) -> Robots:
        async with host.robots_lock:
            if host.robots is None:
                host.robots = await self._load_robots(host, origin)
            return host.robots

    async def _load_robots(self, host: _Host, origin: str) -> Robots:
        try:
            resp = await self._request(host, "GET", f"{origin}/robots.txt", {})
        except FetchError as e:
            # Unreachable robots.txt: don't guess, fail this company tonight.
            raise FetchError(f"robots.txt unavailable for {origin}: {e}") from e
        if resp.status_code == 200:
            return Robots.parse(resp.text)
        if resp.status_code in (401, 403):
            return Robots.disallow_all()
        if 400 <= resp.status_code < 500:
            return Robots.allow_all()  # RFC 9309: missing robots.txt means no rules.
        raise FetchError(f"robots.txt returned HTTP {resp.status_code} for {origin}", resp.status_code)

    async def _request(self, host: _Host, method: str, url: str, headers: dict[str, str],
                       payload: dict | None = None) -> httpx.Response:
        try:
            async for attempt in AsyncRetrying(
                stop=stop_after_attempt(self._max_attempts),
                wait=wait_exponential(multiplier=self._backoff, max=30),
                retry=retry_if_exception_type((httpx.TransportError, _Retryable)),
                reraise=True,
            ):
                with attempt:
                    resp = await self._send(host, method, url, headers, payload)
                    if resp.status_code == 429 or resp.status_code >= 500:
                        raise _Retryable(resp.status_code)
                    return resp
        except _Retryable as e:
            raise FetchError(f"HTTP {e.status} for {url} after {self._max_attempts} attempts", e.status) from e
        except httpx.TransportError as e:
            raise FetchError(f"{type(e).__name__} for {url}") from e
        raise AssertionError("unreachable")

    async def _send(self, host: _Host, method: str, url: str, headers: dict[str, str],
                    payload: dict | None) -> httpx.Response:
        async with host.lock:
            wait = host.next_allowed - time.monotonic()
            if wait > 0:
                await asyncio.sleep(wait)
            try:
                return await self._client.request(method, url, headers=headers, json=payload)
            finally:
                host.next_allowed = time.monotonic() + self._gap(host)

    def _gap(self, host: _Host) -> float:
        gap = random.uniform(self._min_gap, self._max_gap)
        delay = host.robots.crawl_delay(self._token) if host.robots else None
        return max(gap, delay or 0.0)
