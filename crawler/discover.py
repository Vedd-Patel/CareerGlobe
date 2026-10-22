"""Find careers pages and their job boards from company websites.

    python discover.py --websites ../seeds/new.txt [--out ../seeds/discovered.csv]

Input: one company per line, "Name,https://website" or just a website.
Output: rows in the seed file's format, for a human to check and copy into
seeds/companies.csv. City, state and type are left for you to fill in.

For each website, with the crawler's polite client (robots.txt, 1-2 s per host):
1. Try {site}/careers, {site}/jobs, careers.{domain}, jobs.{domain}.
2. On the first page that loads: a known job board in the final URL, links or
   iframes wins; a classic SuccessFactors page or JobPosting JSON-LD comes next;
   otherwise the page itself is kept as a custom careers page.
3. If none loads, fetch the homepage once and follow its "Careers" link.
Boards we have no adapter for yet are still recorded (ats column), so the
crawler reports them by name.
"""

import argparse
import asyncio
import csv
import logging
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from urllib.parse import urljoin, urlsplit

from selectolax.lexbor import LexborHTMLParser

import config
from adapters import detect
from adapters.jsonld import postings
from fetcher import FetchError, PoliteClient

log = logging.getLogger("discover")
FIELDS = ["name", "website", "careers_url", "city", "state", "type", "ats", "note"]
DEFAULT_OUT = config.REPO_ROOT / "seeds" / "discovered.csv"

# Job boards without an adapter yet: (ats name, link pattern). Supported boards
# are found with adapters.detect().
OTHER_BOARDS = [
    ("ashby", re.compile(r"^https?://jobs\.ashbyhq\.com/[^/?#]+", re.I)),
    ("smartrecruiters", re.compile(r"^https?://(careers|jobs)\.smartrecruiters\.com/[^/?#]+", re.I)),
    ("recruitee", re.compile(r"^https?://[a-z0-9-]+\.recruitee\.com", re.I)),
    ("darwinbox", re.compile(r"^https?://[a-z0-9-]+\.darwinbox\.(in|com)", re.I)),
    ("zohorecruit", re.compile(r"^https?://[a-z0-9-]+\.zohorecruit\.(in|com)", re.I)),
    ("freshteam", re.compile(r"^https?://[a-z0-9-]+\.freshteam\.com", re.I)),
    ("icims", re.compile(r"^https?://[a-z0-9-]+\.icims\.com", re.I)),
    ("taleo", re.compile(r"^https?://[a-z0-9-]+\.taleo\.net", re.I)),
    ("successfactors_hosted", re.compile(r"^https?://(career\d*|jobs)\.(sap|successfactors)\.(com|eu)", re.I)),
    ("paramai", re.compile(r"^https?://[a-z0-9-]+\.app\.param\.ai", re.I)),
    ("mynexthire", re.compile(r"^https?://[a-z0-9-]+\.mynexthire\.com", re.I)),
    ("turbohire", re.compile(r"^https?://[a-z0-9-]+\.turbohire\.co", re.I)),
    ("skillate", re.compile(r"^https?://[a-z0-9-]+\.skillate\.com", re.I)),
    ("superset", re.compile(r"^https?://[a-z0-9-]+\.joinsuperset\.com", re.I)),
]
_CAREERS_LINK = re.compile(r"\b(careers?|jobs|join us|work with us|we'?re hiring|openings)\b", re.I)


@dataclass
class Found:
    careers_url: str = ""
    ats: str = ""
    note: str = ""


def read_websites(path: Path) -> list[tuple[str, str]]:
    out = []
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        name, _, site = line.rpartition(",")
        site = site.strip()
        if not site.startswith(("http://", "https://")):
            site = "https://" + site
        host = urlsplit(site).hostname or ""
        out.append((name.strip() or _name_from(host), site.rstrip("/")))
    return out


def candidates(website: str) -> list[str]:
    parts = urlsplit(website)
    host = (parts.hostname or "").lower()
    bare = host[4:] if host.startswith("www.") else host
    origin = f"{parts.scheme or 'https'}://{host}"
    return [f"{origin}/careers", f"{origin}/jobs", f"https://careers.{bare}", f"https://jobs.{bare}"]


def classify(url: str, page_html: str) -> Found | None:
    """What a loaded careers page is, or None if it does not look like one."""
    hit = detect(url)
    if hit:
        return Found(url, hit[0], "board URL")
    tree = LexborHTMLParser(page_html)
    links = [urljoin(url, n.attributes.get(attr) or "")
             for sel, attr in (("a[href]", "href"), ("iframe[src]", "src"), ("script[src]", "src"))
             for n in tree.css(sel)]
    for link in links:
        hit = detect(link)
        if hit:
            return Found(_board_url(link, hit[0]), hit[0], f"linked from {url}")
    for link in links:
        for name, pattern in OTHER_BOARDS:
            m = pattern.match(link)
            if m:
                return Found(m.group(0), name, f"linked from {url}; no adapter yet")
    if tree.css_first("tr.data-row a.jobTitle-link") or "rmkcdn.successfactors.com" in page_html:
        origin = f"{urlsplit(url).scheme}://{urlsplit(url).hostname}/"
        return Found(origin, "successfactors", "classic SuccessFactors site")
    if postings(page_html):
        return Found(url, "", "JobPosting JSON-LD on the page")
    title = tree.css_first("title")
    text = " ".join([title.text() if title else "", url])
    if _CAREERS_LINK.search(text):
        return Found(url, "", "custom careers page: check it lists jobs (JSON-LD/sitemap reader)")
    return None


async def discover_one(client: PoliteClient, name: str, website: str) -> Found:
    for url in candidates(website):
        found = await _try(client, url)
        if found:
            return found
    # Last resort: the homepage, once, for a "Careers" link.
    try:
        home = await client.get(website)
    except FetchError as e:
        return Found(note=f"not found: homepage {e}")
    tree = LexborHTMLParser(home.content.decode("utf-8", "replace"))
    for a in tree.css("a[href]"):
        if _CAREERS_LINK.search(a.text(strip=True) or "") or _CAREERS_LINK.search(a.attributes.get("href") or ""):
            link = urljoin(home.url, a.attributes["href"])
            if detect(link):
                return Found(_board_url(link, detect(link)[0]), detect(link)[0], "linked from homepage")
            found = await _try(client, link)
            if found:
                found.note += " (via homepage link)"
                return found
            break
    return Found(note="not found: no careers page at the usual paths or homepage link")


async def _try(client: PoliteClient, url: str) -> Found | None:
    try:
        res = await client.get(url)
    except FetchError as e:
        log.debug("%s: %s", url, e)
        return None
    return classify(res.url, res.content.decode("utf-8", "replace"))


def _board_url(link: str, ats: str) -> str:
    """A board link trimmed to the board itself (not one job on it)."""
    parts = urlsplit(link)
    if ats in ("greenhouse", "lever", "lever_eu"):
        first = parts.path.strip("/").split("/")[0]
        if parts.path.startswith("/embed/"):
            return link
        return f"{parts.scheme}://{parts.netloc}/{first}"
    if ats == "keka":
        return f"{parts.scheme}://{parts.netloc}/careers/"
    if ats == "workday":
        return f"{parts.scheme}://{parts.netloc}/{detect(link)[1].split('/', 1)[1]}"
    return link


def _name_from(host: str) -> str:
    host = host[4:] if host.startswith("www.") else host
    return host.split(".")[0].replace("-", " ").title()


async def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--websites", type=Path, required=True, help="one 'Name,website' or website per line")
    p.add_argument("--out", type=Path, default=DEFAULT_OUT, help="CSV to write (default: seeds/discovered.csv)")
    args = p.parse_args(argv)
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s", datefmt="%H:%M:%S")
    logging.getLogger("httpx").setLevel(logging.WARNING)
    settings = config.load()

    sites = read_websites(args.websites)
    slots = asyncio.Semaphore(settings.max_hosts_in_flight)
    async with PoliteClient(settings.user_agent, settings.robots_token,
                            settings.min_gap_seconds, settings.max_gap_seconds, timeout=20) as client:
        async def one(name, site):
            async with slots:
                try:
                    found = await discover_one(client, name, site)
                except Exception as e:  # one odd site must not stop the rest
                    found = Found(note=f"not found: {type(e).__name__}")
                log.info("%-28s %-16s %s", name, found.ats or "-", found.careers_url or found.note)
                return name, site, found

        results = await asyncio.gather(*(one(n, s) for n, s in sites))

    with args.out.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=FIELDS)
        w.writeheader()
        for name, site, found in results:
            w.writerow({"name": name, "website": site, "careers_url": found.careers_url, "ats": found.ats,
                        "note": ("discovered: " + found.note) if found.careers_url else found.note})
    ok = sum(1 for *_, f in results if f.careers_url)
    log.info("%d of %d websites have a careers page -> %s", ok, len(results), args.out)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
