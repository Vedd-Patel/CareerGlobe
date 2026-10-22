"""Custom careers pages read with CSS selectors saved by an admin (spec: "Plain
HTML with saved selectors"; S30 "fix selectors" form).

careers_sources.selectors, for example:
    {"item": "div.opening", "title": "h3", "link": "a", "location": ".loc", "next": "a.next"}
- item: one element per job; title / link / location are looked up inside it
  (link defaults to the first <a>, title to the link text).
- next (optional): the next page's link, followed for up to MAX_PAGES pages.
The job's link (canonical URL) is its ID. Seed slug: the careers URL.
"""

from urllib.parse import urljoin

from selectolax.lexbor import LexborHTMLParser

from adapters.base import ParseError, RawJob
from clean import cities_from, work_mode_from
from urls import canonicalize

NAME = "html"
MAX_PAGES = 20


def parse_page(page_html: str, page_url: str, sel: dict) -> tuple[list[RawJob], str | None]:
    """(jobs on this page, next page URL or None)."""
    if not isinstance(sel, dict) or not sel.get("item"):
        raise ParseError("html: selectors need at least an 'item' selector")
    tree = LexborHTMLParser(page_html)
    jobs = []
    for node in tree.css(sel["item"]):
        if sel.get("link"):
            link = node.css_first(sel["link"])
        else:
            link = node if node.tag == "a" else node.css_first("a")
        href = link.attributes.get("href") if link is not None else None
        title_node = node.css_first(sel["title"]) if sel.get("title") else link
        title = title_node.text(strip=True) if title_node is not None else ""
        if not href or not title:
            continue
        url = urljoin(page_url, href)
        loc_node = node.css_first(sel["location"]) if sel.get("location") else None
        location = loc_node.text(strip=True) if loc_node is not None else ""
        jobs.append(RawJob(source_job_id=canonicalize(url), title=title, apply_url=url,
                           cities=cities_from([location]), work_mode=work_mode_from([location])))
    nxt = None
    if sel.get("next"):
        n = tree.css_first(sel["next"])
        if n is not None and n.attributes.get("href"):
            nxt = urljoin(page_url, n.attributes["href"])
    return jobs, nxt


async def fetch(client, careers_url: str, known, selectors=None) -> list[RawJob]:
    jobs: dict[str, RawJob] = {}
    url, seen = careers_url, set()
    for _ in range(MAX_PAGES):
        if url in seen:
            break
        seen.add(url)
        res = await client.get(url)
        page, nxt = parse_page(res.content.decode("utf-8", "replace"), url, selectors)
        for j in page:
            jobs.setdefault(j.source_job_id, j)
        if not nxt:
            break
        url = nxt
    if not jobs:
        raise ParseError("html: the saved selectors found no jobs (page changed?)")
    return list(jobs.values())
