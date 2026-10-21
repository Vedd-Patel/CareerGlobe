"""Heartbeat pings to a free monitor (healthchecks.io), so a missed night emails you.

The ping URL comes from the environment (e.g. HEALTHCHECK_CRAWL_URL). Empty or
unset: every call does nothing. A ping that fails is logged and ignored; it
never fails the job it reports on. The URL is a secret-ish token: never logged.
"""

import logging
import os

import httpx

log = logging.getLogger(__name__)
MAX_BODY = 10_000  # healthchecks.io keeps the first 10 kB of a ping body


class Heartbeat:
    def __init__(self, env_var: str, user_agent: str, timeout: float = 10.0):
        self.url = (os.environ.get(env_var) or "").strip().rstrip("/")
        self.env_var, self.user_agent, self.timeout = env_var, user_agent, timeout

    @property
    def enabled(self) -> bool:
        return bool(self.url)

    async def start(self) -> None:
        await self._ping("/start", "")

    async def success(self, body: str = "") -> None:
        await self._ping("", body)

    async def fail(self, body: str = "") -> None:
        await self._ping("/fail", body)

    async def _ping(self, suffix: str, body: str) -> None:
        if not self.enabled:
            return
        try:
            async with httpx.AsyncClient(timeout=self.timeout, headers={"User-Agent": self.user_agent}) as c:
                resp = await c.post(self.url + suffix, content=body[:MAX_BODY].encode())
            if resp.status_code >= 400:
                log.warning("heartbeat %s%s: HTTP %d", self.env_var, suffix, resp.status_code)
        except httpx.HTTPError as e:
            log.warning("heartbeat %s%s failed: %s", self.env_var, suffix, type(e).__name__)
