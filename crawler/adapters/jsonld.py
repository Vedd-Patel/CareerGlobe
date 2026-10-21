"""Custom careers pages: schema.org JobPosting JSON-LD, found via the sitemap.

The fallback for any careers page that is not a known ATS. Seed slug: the
careers URL itself.

1. The careers page: any JobPosting objects in its JSON-LD are jobs already.
2. The careers host's sitemaps (robots.txt Sitemap: lines, else /sitemap.xml;
   indexes followed, same host only): URLs under the careers path or with a
   job-like path segment are candidate job pages.
3. A candidate page is fetched only when it is new to us or its <lastmod>
   changed (at most MAX_NEW_DETAILS a night); unchanged ones are kept as
   partial. A fetched page counts as a job only if it has a JobPosting that
   has not expired (validThrough) and is not clearly outside India.

Nothing found at all (no JobPosting on the page, no job URLs in the sitemap)
fails the company, so a redesigned site never closes jobs.
Microdata-only pages are not read (decision 2026-10-21: no extruct).
"""

import gzip
import html
import json
import logging
import re
from datetime import datetime, timezone
from urllib.parse import urljoin, urlsplit

from selectolax.lexbor import LexborHTMLParser

from adapters.base import ParseError, RawJob
from clean import annual_inr, cities_from, clean_html, work_mode_from
from fetcher import FetchError
from urls import canonicalize

log = logging.getLogger(__name__)
NAME = "jsonld"
MAX_NEW_DETAILS = 300
MAX_SITEMAPS = 20
MAX_JOB_URLS = 3000

_JOBISH = re.compile(r"/(jobs?|careers?|openings?|positions?|vacanc\w*|requisitions?)/", re.IGNORECASE)
_INDIA = {"in", "ind", "india"}
_PERIODS = {"YEAR": "year", "MONTH": "month"}
_URL_BLOCK = re.compile(r"<(url|sitemap)>(.*?)</\1>", re.S | re.IGNORECASE)
_TAG = {k: re.compile(rf"<{k}>\s*(.*?)\s*</{k}>", re.S | re.IGNORECASE) for k in ("loc", "lastmod")}


# --- JSON-LD ------------------------------------------------------------------------------

def postings(page_html: str) -> list[dict]:
    """Every JobPosting object in the page's JSON-LD (lists, @graph, ItemList)."""
    out: list[dict] = []
    for script in LexborHTMLParser(page_html).css('script[type="application/ld+json"]'):
        try:
            data = json.loads(script.text(), strict=False)
        except ValueError:
            continue
        _collect(data, out)
    return out


def _collect(node, out: list[dict]) -> None:
    if isinstance(node, list):
        for x in node:
            _collect(x, out)
    elif isinstance(node, dict):
        types = node.get("@type")
        types = types if isinstance(types, list) else [types]
        if "JobPosting" in types:
            out.append(node)
            return
        for key in ("@graph", "itemListElement", "item", "mainEntity"):
            if key in node:
                _collect(node[key], out)


def to_job(p: dict, page_url: str, now: datetime | None = None) -> RawJob | None:
    """A RawJob from one JobPosting, or None when it is not a usable open job in India."""
    title = _text(p.get("title") or p.get("name"))
    if not title:
        return None
    url = urljoin(page_url, _text(p.get("url")) or page_url)
    valid = _dt(p.get("validThrough"))
    if valid and valid < (now or datetime.now(timezone.utc)):
        return None  # expired

    places, countries = [], []
    for loc in _as_list(p.get("jobLocation")):
        addr = loc.get("address") if isinstance(loc, dict) else None
        if isinstance(addr, str):
            places.append(addr)
        elif isinstance(addr, dict):
            places.append(", ".join(_text(addr.get(k)) for k in ("addressLocality", "addressRegion") if addr.get(k)))
            if addr.get("addressCountry"):
                countries.append(_text(addr["addressCountry"]))
    remote = "TELECOMMUTE" in str(p.get("jobLocationType") or "").upper()
    for req in _as_list(p.get("applicantLocationRequirements")):
        countries.append(_text(req))
    if countries and not any(c.strip().lower() in _INDIA for c in countries):
        return None  # clearly outside India

    salary = p.get("baseSalary") if isinstance(p.get("baseSalary"), dict) else {}
    value = salary.get("value") if isinstance(salary.get("value"), dict) else {}
    lo = value.get("minValue", value.get("value"))
    hi = value.get("maxValue", value.get("value"))
    lo, hi = annual_inr(lo, hi, salary.get("currency") or value.get("currency"),
                       _PERIODS.get(str(value.get("unitText") or salary.get("unitText") or "").upper()))

    description = p.get("description")
    return RawJob(
        source_job_id=canonicalize(url),
        title=title,
        apply_url=url,
        description=clean_html(html.unescape(description)) if isinstance(description, str) else None,
        cities=cities_from(places),
        work_mode="remote" if remote else work_mode_from(places),
        salary_min=lo,
        salary_max=hi,
        posted_at=_dt(p.get("datePosted")),
    )


# --- Sitemaps -----------------------------------------------------------------------------

def parse_sitemap(body: bytes) -> tuple[list[tuple[str, str | None]], list[str]]:
    """([(page URL, lastmod)], [child sitemap URLs]) from a urlset or sitemapindex."""
    if body[:2] == b"\x1f\x8b":
        body = gzip.decompress(body)
    text = body.decode("utf-8", "replace")
    pages, children = [], []
    for kind, block in _URL_BLOCK.findall(text):
        loc = _TAG["loc"].search(block)
        if not loc:
            continue
        url = html.unescape(loc.group(1).strip())
        lastmod = _TAG["lastmod"].search(block)
        if kind.lower() == "sitemap":
            children.append(url)
        else:
            pages.append((url, lastmod.group(1).strip() if lastmod else None))
    return pages, children


def job_like(url: str, careers_url: str) -> bool:
    """Same host as the careers page, and under its path or with a job-like segment."""
    u, c = urlsplit(url), urlsplit(careers_url)
    if _bare(u.hostname) != _bare(c.hostname) or u.path.endswith((".xml", ".xml.gz", ".pdf")):
        return False
    path, base = u.path.rstrip("/"), c.path.rstrip("/")
    if path == base or not path:
        return False
    return (bool(base) and path.startswith(base + "/")) or bool(_JOBISH.search(path + "/"))


async def sitemap_pages(client, careers_url: str) -> list[tuple[str, str | None]]:
    parts = urlsplit(careers_url)
    origin = f"{parts.scheme}://{parts.netloc}"
    queue = [u for u in await client.sitemaps(careers_url) if _bare(urlsplit(u).hostname) == _bare(parts.hostname)]
    queue = queue or [f"{origin}/sitemap.xml"]
    seen, pages = set(), {}
    while queue and len(seen) < MAX_SITEMAPS:
        url = queue.pop(0)
        if url in seen:
            continue
        seen.add(url)
        try:
            res = await client.get(url)
        except FetchError as e:
            if e.status in (404, 410):
                continue
            raise
        found, children = parse_sitemap(res.content)
        for page, lastmod in found:
            if job_like(page, careers_url):
                pages.setdefault(page, lastmod)
        children = [c for c in children if _bare(urlsplit(c).hostname) == _bare(parts.hostname)]
        # In an index, prefer job/career sitemaps when there are any.
        preferred = [c for c in children if re.search(r"job|career|position|vacanc", c, re.IGNORECASE)]
        queue += preferred or children
    if len(pages) > MAX_JOB_URLS:
        raise ParseError(f"jsonld: {len(pages)} job-like sitemap URLs (> {MAX_JOB_URLS}); path filter too loose?")
    return list(pages.items())


# --- One company --------------------------------------------------------------------------

async def fetch(client, careers_url: str, known) -> list[RawJob]:
    res = await client.get(careers_url)
    jobs: dict[str, RawJob] = {}
    for p in postings(res.content.decode("utf-8", "replace")):
        if job := to_job(p, careers_url):
            jobs.setdefault(job.source_job_id, job)

    candidates = await sitemap_pages(client, careers_url)
    if not jobs and not candidates:
        raise ParseError("jsonld: no JobPosting on the careers page and no job URLs in its sitemap")

    fetched = with_posting = deferred = 0
    for url, lastmod in candidates:
        key = canonicalize(url)
        if key in jobs:
            continue
        unchanged = lastmod is None or lastmod == known.get(key)
        if key in known and (unchanged or fetched >= MAX_NEW_DETAILS):
            # Over the cap a changed page waits a night, but its job stays open.
            jobs[key] = RawJob(source_job_id=key, title="", apply_url=url, partial=True)
            deferred += not unchanged
            continue
        if fetched >= MAX_NEW_DETAILS:
            deferred += 1
            continue
        fetched += 1
        try:
            page = await client.get(url)
        except FetchError as e:
            if e.status in (404, 410):
                continue
            raise
        found = postings(page.content.decode("utf-8", "replace"))
        if found:
            with_posting += 1
        job = to_job(found[0], url) if found else None
        if job:
            job.source_job_id, job.apply_url, job.source_updated = key, url, lastmod
            jobs[key] = job
    if fetched and not with_posting and not jobs and not known:
        raise ParseError(f"jsonld: none of {fetched} sitemap pages has a JobPosting")
    if deferred:
        log.info("jsonld %s: %d pages left for the next night (cap %d)", careers_url, deferred, MAX_NEW_DETAILS)
    return list(jobs.values())


def _bare(host: str | None) -> str:
    host = (host or "").lower()
    return host[4:] if host.startswith("www.") else host


def _as_list(v) -> list:
    return v if isinstance(v, list) else ([v] if v else [])


def _text(v) -> str:
    if isinstance(v, dict):
        v = v.get("name") or v.get("@value") or v.get("value") or ""
    return html.unescape(str(v)).strip() if v else ""


def _dt(value) -> datetime | None:
    if not value or not isinstance(value, str):
        return None
    try:
        d = datetime.fromisoformat(value.strip().replace("Z", "+00:00"))
    except ValueError:
        return None
    return d if d.tzinfo else d.replace(tzinfo=timezone.utc)
