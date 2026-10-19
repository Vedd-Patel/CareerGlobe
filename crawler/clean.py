"""Cleaning what comes back: cities, work mode, salary, description HTML.

City aliases live here until the admin-editable city_aliases table arrives
(Phase 2). Keep this list small: unknown cities still pass through as-is.
"""

import html
import re
from html.parser import HTMLParser

MAX_DESCRIPTION_CHARS = 20_000

# lowercase alias -> canonical name
CITY_ALIASES = {
    "bengaluru": "Bengaluru", "bangalore": "Bengaluru", "blr": "Bengaluru",
    "gurugram": "Gurugram", "gurgaon": "Gurugram",
    "mumbai": "Mumbai", "bombay": "Mumbai", "navi mumbai": "Navi Mumbai", "thane": "Thane",
    "delhi": "Delhi", "new delhi": "Delhi",
    "noida": "Noida", "greater noida": "Greater Noida",
    "hyderabad": "Hyderabad", "secunderabad": "Hyderabad",
    "chennai": "Chennai", "madras": "Chennai",
    "kolkata": "Kolkata", "calcutta": "Kolkata",
    "pune": "Pune", "ahmedabad": "Ahmedabad", "jaipur": "Jaipur",
    "kochi": "Kochi", "cochin": "Kochi",
    "thiruvananthapuram": "Thiruvananthapuram", "trivandrum": "Thiruvananthapuram",
    "coimbatore": "Coimbatore", "indore": "Indore", "chandigarh": "Chandigarh",
    "mohali": "Mohali", "lucknow": "Lucknow", "bhubaneswar": "Bhubaneswar",
    "nagpur": "Nagpur", "vadodara": "Vadodara", "baroda": "Vadodara", "surat": "Surat",
    "mysuru": "Mysuru", "mysore": "Mysuru", "mangaluru": "Mangaluru", "mangalore": "Mangaluru",
    "visakhapatnam": "Visakhapatnam", "vizag": "Visakhapatnam", "goa": "Goa",
}
# Longest first, so "new delhi" wins over "delhi" and "navi mumbai" over "mumbai".
_CITY_RE = re.compile(
    r"\b(" + "|".join(re.escape(a) for a in sorted(CITY_ALIASES, key=len, reverse=True)) + r")\b",
    re.IGNORECASE,
)
# Location strings that are not a city.
NOT_A_CITY = {
    "india", "remote", "multiple", "multiple locations", "pan india", "anywhere", "hybrid",
    "onsite", "on-site", "work from home", "wfh", "registered office", "",
}


def cities_from(locations: list[str]) -> list[str]:
    """Known Indian cities found anywhere in the strings; otherwise the first
    comma part of each string if it looks like a place. Never guesses one city
    for "India" or "Multiple locations"."""
    found: list[str] = []
    for loc in locations:
        if not loc:
            continue
        hits = [CITY_ALIASES[m.lower()] for m in _CITY_RE.findall(loc)]
        if not hits:
            first = re.split(r"[,(]", loc, maxsplit=1)[0].strip()
            if first.lower() not in NOT_A_CITY:
                hits = [first]
        for city in hits:
            if city not in found:
                found.append(city)
    return found


def work_mode_from(texts: list[str]) -> str | None:
    joined = " ".join(t for t in texts if t).lower()
    if "hybrid" in joined:
        return "hybrid"
    if "remote" in joined or "work from home" in joined:
        return "remote"
    return None


def annual_inr(low, high, currency: str | None, period: str | None) -> tuple[int | None, int | None]:
    """Salary as annual INR, only when the posting states it in INR per year or
    per month. Anything else (other currency, hourly, missing) is left empty."""
    if (currency or "").upper() != "INR" or period not in ("year", "month"):
        return None, None
    factor = 12 if period == "month" else 1

    def conv(v):
        try:
            v = float(v)
        except (TypeError, ValueError):
            return None
        return int(round(v * factor)) if v > 0 else None

    lo, hi = conv(low), conv(high)
    if lo and hi and lo > hi:
        lo, hi = hi, lo
    return lo, hi


_ALLOWED = {"p", "br", "ul", "ol", "li", "strong", "b", "em", "i", "u",
            "h1", "h2", "h3", "h4", "h5", "h6", "a", "blockquote"}
_RENAME = {"div": "p"}
_DROP_WITH_CONTENT = {"script", "style", "iframe", "noscript", "svg", "object", "embed", "template"}
_VOID = {"br"}


class _Sanitizer(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.out: list[str] = []
        self._skip = 0

    def handle_starttag(self, tag, attrs):
        if tag in _DROP_WITH_CONTENT:
            self._skip += 1
            return
        if self._skip:
            return
        tag = _RENAME.get(tag, tag)
        if tag not in _ALLOWED:
            return
        if tag == "a":
            href = dict(attrs).get("href") or ""
            if href.startswith(("http://", "https://", "mailto:")):
                self.out.append(f'<a href="{html.escape(href, quote=True)}" rel="nofollow noopener">')
            else:
                self.out.append("<a>")
        else:
            self.out.append(f"<{tag}>")

    def handle_startendtag(self, tag, attrs):
        if not self._skip and _RENAME.get(tag, tag) in _VOID:
            self.out.append("<br>")

    def handle_endtag(self, tag):
        if tag in _DROP_WITH_CONTENT:
            self._skip = max(0, self._skip - 1)
            return
        if self._skip:
            return
        tag = _RENAME.get(tag, tag)
        if tag in _ALLOWED and tag not in _VOID:
            self.out.append(f"</{tag}>")

    def handle_data(self, data):
        if not self._skip:
            self.out.append(html.escape(data, quote=False))


def clean_html(raw: str | None) -> str | None:
    """Strip scripts, styles and attributes; keep basic formatting; cap length."""
    if not raw:
        return None
    s = _Sanitizer()
    s.feed(raw)
    s.close()
    out = "".join(s.out)
    out = re.sub(r"<p>(\s|&nbsp;|<br>)*</p>", "", out)
    out = re.sub(r"\n{3,}", "\n\n", out).strip()
    if len(out) > MAX_DESCRIPTION_CHARS:
        cut = out[:MAX_DESCRIPTION_CHARS]
        # Never end inside a tag or an entity.
        lt, amp = cut.rfind("<"), cut.rfind("&")
        if lt > cut.rfind(">"):
            cut = cut[:lt]
        if amp > cut.rfind(";"):
            cut = cut[:amp]
        out = cut
    return out or None
