"""ATS adapters and detection from a careers URL.

Each adapter has board_url(slug) and parse(data, slug) -> list[RawJob].
"""

import re
from dataclasses import dataclass
from typing import Callable
from urllib.parse import parse_qs, urlsplit

from adapters import greenhouse, keka, lever
from adapters.base import ParseError, RawJob


@dataclass(frozen=True)
class Adapter:
    name: str
    board_url: Callable[[str], str]
    parse: Callable[[object, str], list[RawJob]]


ADAPTERS = {
    "greenhouse": Adapter("greenhouse", greenhouse.board_url, greenhouse.parse),
    "lever": Adapter("lever", lever.board_url, lever.parse),
    "lever_eu": Adapter("lever_eu", lever.board_url_eu, lever.parse),
    "keka": Adapter("keka", keka.board_url, keka.parse),
}

_SLUG = r"([A-Za-z0-9][A-Za-z0-9_.-]*)"
_PATTERNS = [
    ("greenhouse", re.compile(r"^(?:boards|job-boards)(?:\.eu)?\.greenhouse\.io$"), re.compile(rf"^/{_SLUG}")),
    ("greenhouse", re.compile(r"^boards-api(?:\.eu)?\.greenhouse\.io$"), re.compile(rf"^/v1/boards/{_SLUG}")),
    ("lever", re.compile(r"^jobs\.lever\.co$"), re.compile(rf"^/{_SLUG}")),
    ("lever_eu", re.compile(r"^jobs\.eu\.lever\.co$"), re.compile(rf"^/{_SLUG}")),
]
_KEKA_HOST = re.compile(r"^([a-z0-9-]+)\.keka\.com$")


def detect(careers_url: str) -> tuple[str, str] | None:
    """(ats_type, ats_slug) when the careers URL is itself an ATS board.
    Custom careers pages that only link to an ATS are discover.py's job (Phase 2)."""
    parts = urlsplit(careers_url.strip())
    host = (parts.hostname or "").lower()
    m = _KEKA_HOST.match(host)
    if m and m.group(1) not in ("www", "app"):
        return "keka", m.group(1)
    if re.match(r"^(?:boards|job-boards)(?:\.eu)?\.greenhouse\.io$", host) and parts.path.startswith("/embed/"):
        board = parse_qs(parts.query).get("for", [""])[0]
        return ("greenhouse", board) if board else None
    for ats, host_re, path_re in _PATTERNS:
        if host_re.match(host):
            pm = path_re.match(parts.path)
            if pm:
                return ats, pm.group(1)
    return None


__all__ = ["ADAPTERS", "Adapter", "ParseError", "RawJob", "detect"]
