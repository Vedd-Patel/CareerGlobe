"""Company logos: the icon each company's own website names for itself.

The home page (the seed file's website column) is read once, politely (robots.txt,
1-2 s gap, honest user agent), for its <link rel="apple-touch-icon"> and
<link rel="icon">. The best candidate that really is an image is stored as a
link, never the image itself: the app shows it straight from the company's site.
A company without one keeps the lettered badge and is looked at again in a month.
"""

import logging
import re
from urllib.parse import urljoin, urlparse

from selectolax.lexbor import LexborHTMLParser

from fetcher import FetchError, is_private_host

log = logging.getLogger(__name__)
PER_RUN = 40
RECHECK_DAYS = 30
MAX_BYTES = 600_000
TRIES = 3  # candidates tried per company


def candidates(html: str, base_url: str) -> list[str]:
    """Icon URLs from the page, best first: touch icons by size, then vector and large icons, then the rest."""
    scored: list[tuple[int, str]] = []
    for link in LexborHTMLParser(html).css("link[rel][href]"):
        rels = set((link.attributes.get("rel") or "").lower().split())
        href = (link.attributes.get("href") or "").strip()
        if not href or href.startswith("data:"):
            continue
        url = urljoin(base_url, href)
        parsed = urlparse(url)
        if parsed.scheme != "https" or not parsed.hostname or is_private_host(parsed.hostname):
            continue  # a page must not point the user's browser at a plain-http or private address
        size = 0
        m = re.search(r"(\d+)x(\d+)", link.attributes.get("sizes") or "")
        if m:
            size = int(m.group(1))
        if "apple-touch-icon" in rels or "apple-touch-icon-precomposed" in rels:
            scored.append((3000 + size, url))
        elif rels & {"icon", "shortcut"}:
            vector = url.lower().split("?")[0].endswith(".svg") or "svg" in (link.attributes.get("type") or "")
            scored.append(((2000 if vector else 1000) + size, url))
    seen, out = set(), []
    for _, url in sorted(scored, key=lambda t: -t[0]):
        if url not in seen:
            seen.add(url)
            out.append(url)
    return out


def looks_like_image(data: bytes) -> bool:
    """PNG, JPEG, GIF, WebP, ICO or SVG, by their first bytes (the fetcher does not expose content types)."""
    if not data or len(data) > MAX_BYTES:
        return False
    head = data[:600].lstrip()
    return (data.startswith((b"\x89PNG", b"\xff\xd8\xff", b"GIF8", b"\x00\x00\x01\x00"))
            or (data[:4] == b"RIFF" and data[8:12] == b"WEBP")
            or b"<svg" in head.lower())


async def run(conn, client, limit: int = PER_RUN) -> tuple[int, int]:
    """Look up to `limit` companies without a logo. Returns (looked at, logos found)."""
    cur = await conn.execute(
        """SELECT id, website FROM companies
           WHERE logo_url IS NULL AND website IS NOT NULL AND website <> ''
             AND (logo_checked_at IS NULL OR logo_checked_at < now() - make_interval(days => %s))
           ORDER BY id LIMIT %s""",
        (RECHECK_DAYS, limit),
    )
    checked = found = 0
    for company_id, website in await cur.fetchall():
        logo = None
        try:
            page = await client.get(website)
            html = page.content.decode("utf-8", "replace")
            for url in candidates(html, page.url or website)[:TRIES]:
                try:
                    icon = await client.get(url)
                except FetchError:
                    continue
                if looks_like_image(icon.content):
                    logo = url
                    break
        except FetchError as e:
            log.info("no logo for %s: %s", website, e)
        checked += 1
        found += logo is not None
        await conn.execute("UPDATE companies SET logo_url = %s, logo_checked_at = now() WHERE id = %s", (logo, company_id))
    return checked, found
