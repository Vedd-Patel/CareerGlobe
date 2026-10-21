"""robots.txt rules per RFC 9309.

Python's urllib.robotparser applies the first matching rule in file order.
RFC 9309 (and Google) apply the most specific (longest) match, with Allow
winning ties. Keka's "Disallow: /" then "Allow: /careers" only works with the
RFC behaviour, so we parse it ourselves.
"""

import re
from dataclasses import dataclass, field
from urllib.parse import urlsplit


@dataclass
class _Group:
    agents: list[str] = field(default_factory=list)
    rules: list[tuple[bool, str]] = field(default_factory=list)  # (allow, pattern)
    crawl_delay: float | None = None


class Robots:
    def __init__(self, groups: list[_Group], sitemaps: list[str] | None = None):
        self._groups = groups
        self.sitemaps = sitemaps or []  # Sitemap: lines (not tied to any group)

    @classmethod
    def allow_all(cls) -> "Robots":
        return cls([])

    @classmethod
    def disallow_all(cls) -> "Robots":
        return cls([_Group(agents=["*"], rules=[(False, "/")])])

    @classmethod
    def parse(cls, text: str) -> "Robots":
        groups: list[_Group] = []
        sitemaps: list[str] = []
        current: _Group | None = None
        last_was_agent = False
        for raw in text.splitlines():
            line = raw.split("#", 1)[0].strip()
            if ":" not in line:
                continue
            key, value = (s.strip() for s in line.split(":", 1))
            key = key.lower()
            if key == "sitemap":
                if value:
                    sitemaps.append(value)
                continue
            if key == "user-agent":
                if current is None or not last_was_agent:
                    current = _Group()
                    groups.append(current)
                current.agents.append(value.lower())
                last_was_agent = True
                continue
            last_was_agent = False
            if current is None:
                continue
            if key in ("allow", "disallow") and value:
                current.rules.append((key == "allow", value))
            elif key == "crawl-delay":
                try:
                    current.crawl_delay = float(value)
                except ValueError:
                    pass
        return cls(groups, sitemaps)

    def _groups_for(self, token: str) -> list[_Group]:
        token = token.lower()
        specific = [g for g in self._groups if token in g.agents]
        return specific or [g for g in self._groups if "*" in g.agents]

    def can_fetch(self, token: str, url: str) -> bool:
        parts = urlsplit(url)
        path = (parts.path or "/") + (f"?{parts.query}" if parts.query else "")
        if path == "/robots.txt":
            return True
        best_len, allowed = -1, True
        for group in self._groups_for(token):
            for allow, pattern in group.rules:
                if _matches(pattern, path):
                    n = len(pattern)
                    if n > best_len or (n == best_len and allow):
                        best_len, allowed = n, allow
        return allowed

    def crawl_delay(self, token: str) -> float | None:
        delays = [g.crawl_delay for g in self._groups_for(token) if g.crawl_delay is not None]
        return max(delays) if delays else None


def _matches(pattern: str, path: str) -> bool:
    anchored = pattern.endswith("$")
    body = pattern[:-1] if anchored else pattern
    regex = ".*".join(re.escape(part) for part in body.split("*"))
    return re.match(regex + ("$" if anchored else ""), path) is not None
