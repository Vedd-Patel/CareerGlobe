"""Company logos: the icon each company names for itself.

1. The home page (the seed file's website column) is read once, politely (robots.txt,
   1-2 s gap, honest user agent), for its <link rel="apple-touch-icon"> and
   <link rel="icon">; failing that, the standard /apple-touch-icon.png and /favicon.ico.
2. Failing that, the company's own careers page from the seed file: a Keka board shows
   the company's logo, a Lever board's og:image is its logo, an Ashby board's og:image is
   its logo. A board's own favicon is the job board's, not the company's, so it is never used.
   A careers page on the company's own site is read like a home page.
The best candidate that really is an image is stored as a link, never the image itself:
the app shows it straight from where it is hosted. A company without one keeps the
lettered badge and is looked at again in a month.
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


# Job-board and recruiting-software domains: their icons are the vendor's, never a company's.
VENDOR_HOSTS = ("keka.com", "lever.co", "greenhouse.io", "ashbyhq.com", "workable.com", "myworkdayjobs.com",
                "myworkdaysite.com", "oraclecloud.com", "successfactors.com", "successfactors.eu", "eightfold.ai",
                "smartrecruiters.com", "icims.com", "taleo.net", "recruitee.com", "darwinbox.in", "zohorecruit.com")
# Where a vendor serves company logos from, so one is trusted without fetching it again.
LOGO_HOSTS = ("lever-client-logos.s3.us-west-2.amazonaws.com", "app.ashbyhq.com")


def is_vendor(host: str) -> bool:
    host = host.lower()
    return any(host == v or host.endswith("." + v) for v in VENDOR_HOSTS)


def board_logo(html: str, page_url: str) -> str | None:
    """The company's logo from a Keka, Lever or Ashby board page, or None."""
    host = (urlparse(page_url).hostname or "").lower()
    tree = LexborHTMLParser(html)
    url = None
    if host.endswith(".keka.com"):
        for img in tree.css("img.org-logo[src]"):
            src = (img.attributes.get("src") or "").strip()
            if "/orglogo/" in src:
                url = urljoin(page_url, src)
                break
    elif host == "jobs.lever.co" or host == "jobs.eu.lever.co":
        meta = tree.css_first('meta[property="og:image"]')
        url = (meta.attributes.get("content") or "").strip() if meta else None
        if url and urlparse(url).hostname not in LOGO_HOSTS:
            url = None
    elif host == "jobs.ashbyhq.com":
        meta = tree.css_first('meta[property="og:image"]')
        url = (meta.attributes.get("content") or "").strip() if meta else None
        if url and "/org-theme-logo/" not in url:
            url = None
    if not url:
        return None
    parsed = urlparse(url)
    if parsed.scheme != "https" or not parsed.hostname or is_private_host(parsed.hostname):
        return None
    return url


def standard_icons(base_url: str) -> list[str]:
    """The conventional icon locations, tried when a page names none."""
    return [urljoin(base_url, "/apple-touch-icon.png"), urljoin(base_url, "/favicon.ico")]


def looks_like_image(data: bytes) -> bool:
    """PNG, JPEG, GIF, WebP, ICO or SVG, by their first bytes (the fetcher does not expose content types)."""
    if not data or len(data) > MAX_BYTES:
        return False
    head = data[:600].lstrip()
    return (data.startswith((b"\x89PNG", b"\xff\xd8\xff", b"GIF8", b"\x00\x00\x01\x00"))
            or (data[:4] == b"RIFF" and data[8:12] == b"WEBP")
            or b"<svg" in head.lower())


async def _first_image(client, urls: list[str]) -> str | None:
    for url in urls[:TRIES]:
        try:
            icon = await client.get(url)
        except FetchError:
            continue
        if looks_like_image(icon.content):
            return url
    return None


async def _logo_from_page(client, url: str) -> str | None:
    """A logo for one page: its board logo (Keka, Lever, Ashby), or its icons when it is on the company's own site."""
    page = await client.get(url)
    html = page.content.decode("utf-8", "replace")
    final = page.url or url
    host = (urlparse(final).hostname or "").lower()
    if is_vendor(host):
        logo = board_logo(html, final)
        logo_host = (urlparse(logo).hostname or "") if logo else ""
        if logo and (logo_host in LOGO_HOSTS or (logo_host.endswith(".keka.com") and "/orglogo/" in logo)):
            # Named by the board page itself and served by the vendor's own store. Keka's robots.txt
            # allows only /careers, so the image is not fetched here; the member's browser loads it.
            return logo
        return logo if logo and await _first_image(client, [logo]) else None
    found = candidates(html, final)
    return await _first_image(client, found or standard_icons(final))


async def run(conn, client, limit: int = PER_RUN) -> tuple[int, int]:
    """Look up to `limit` companies without a logo. Returns (looked at, logos found)."""
    cur = await conn.execute(
        """SELECT c.id, c.website,
                  (SELECT s.careers_url FROM careers_sources s WHERE s.company_id = c.id AND s.status <> 'disabled'
                   ORDER BY s.id LIMIT 1)
           FROM companies c
           WHERE c.logo_url IS NULL
             AND (c.logo_checked_at IS NULL OR c.logo_checked_at < now() - make_interval(days => %s))
           ORDER BY c.id LIMIT %s""",
        (RECHECK_DAYS, limit),
    )
    checked = found = 0
    for company_id, website, careers_url in await cur.fetchall():
        logo = None
        for url in [u for u in (website, careers_url) if u]:
            try:
                logo = await _logo_from_page(client, url)
            except FetchError as e:
                log.info("no logo from %s: %s", url, e)
            if logo:
                break
        checked += 1
        found += logo is not None
        await conn.execute("UPDATE companies SET logo_url = %s, logo_checked_at = now() WHERE id = %s", (logo, company_id))
    return checked, found
