"""Crawler settings, read from environment variables (and the repo's .env)."""

import os
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv

REPO_ROOT = Path(__file__).resolve().parent.parent


@dataclass(frozen=True)
class Settings:
    database_url: str | None
    user_agent: str
    min_gap_seconds: float
    max_gap_seconds: float
    max_hosts_in_flight: int
    stop_at_ist: str

    @property
    def robots_token(self) -> str:
        """Product token matched against robots.txt User-agent lines."""
        return self.user_agent.split("/", 1)[0].strip()


def load() -> Settings:
    # Real environment variables win over .env.
    load_dotenv(REPO_ROOT / ".env", override=False)
    user_agent = os.environ.get("CRAWLER_USER_AGENT", "").strip()
    if not user_agent:
        raise SystemExit("CRAWLER_USER_AGENT must be set (see .env.example)")
    return Settings(
        database_url=os.environ.get("CRAWLER_DATABASE_URL") or os.environ.get("DATABASE_URL"),
        user_agent=user_agent,
        min_gap_seconds=float(os.environ.get("CRAWLER_MIN_GAP_SECONDS", "1.0")),
        max_gap_seconds=float(os.environ.get("CRAWLER_MAX_GAP_SECONDS", "2.0")),
        max_hosts_in_flight=int(os.environ.get("CRAWLER_MAX_HOSTS_IN_FLIGHT", "32")),
        stop_at_ist=os.environ.get("CRAWLER_STOP_AT_IST", "05:30"),
    )
