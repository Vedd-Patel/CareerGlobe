"""Find job boards by company name on the ATSs with public APIs.

    python probe_boards.py --names ../seeds/names.txt [--out ../seeds/discovered_boards.csv]

Input: one company name per line ("Name" or "Name,website"). For each name a few
slug spellings are tried on Greenhouse, Lever, Workable and Keka (one request
each, through the polite client: robots.txt, 1-2 s per host). A board is kept
only when it has jobs and at least MIN_INDIA_SHARE of its located jobs are in
India, so a foreign company with the same name is not picked up. Output rows
are in the seed file's format, with the city most common among the jobs.
"""

import argparse
import asyncio
import csv
import logging
import re
import sys
from collections import Counter
from dataclasses import dataclass
from pathlib import Path

import config
from adapters import ADAPTERS
from fetcher import FetchError, PoliteClient

log = logging.getLogger("probe")
MIN_INDIA_SHARE = 0.3
FIELDS = ["name", "website", "careers_url", "city", "state", "type", "ats", "note"]
PUBLIC_URL = {
    "greenhouse": "https://boards.greenhouse.io/{slug}",
    "lever": "https://jobs.lever.co/{slug}",
    "workable": "https://apply.workable.com/{slug}/",
    "keka": "https://{slug}.keka.com/careers/",
}
ORDER = ["greenhouse", "lever", "workable", "keka"]


@dataclass
class Hit:
    ats: str
    slug: str
    jobs: int
    india: int
    city: str


def slugs(name: str) -> list[str]:
    """'Urban Company' -> ['urbancompany', 'urban-company']; drops Pvt/Ltd/Technologies."""
    base = re.sub(r"\b(private|pvt|limited|ltd|llp|inc|technologies|technology|labs|india)\b\.?", " ", name.lower())
    words = re.findall(r"[a-z0-9]+", base) or re.findall(r"[a-z0-9]+", name.lower())
    out = ["".join(words), "-".join(words)]
    full = re.findall(r"[a-z0-9]+", name.lower())
    out += ["".join(full), "-".join(full)]
    seen, result = set(), []
    for s in out:
        if s and len(s) >= 3 and s not in seen:
            seen.add(s)
            result.append(s)
    return result


async def try_board(client: PoliteClient, ats: str, slug: str) -> Hit | None:
    adapter = ADAPTERS[ats]
    try:
        res = await client.get(adapter.board_url(slug))
        jobs = adapter.parse(res.json(), slug)
    except (FetchError, ValueError, Exception) as e:  # 404s and odd pages mean "no board here"
        log.debug("%s %s: %s", ats, slug, type(e).__name__)
        return None
    located = [j for j in jobs if j.in_india is not None]
    india = [j for j in jobs if j.in_india]
    if not jobs or not india or (located and len(india) / len(located) < MIN_INDIA_SHARE):
        return None
    cities = Counter(c for j in india for c in j.cities)
    return Hit(ats, slug, len(jobs), len(india), cities.most_common(1)[0][0] if cities else "")


async def probe(client: PoliteClient, name: str) -> Hit | None:
    for ats in ORDER:
        for slug in slugs(name):
            hit = await try_board(client, ats, slug)
            if hit:
                return hit
    return None


def read_names(path: Path, skip: set[str]) -> list[tuple[str, str]]:
    out = []
    for raw in path.read_text(encoding="utf-8").splitlines():
        line = raw.split("#", 1)[0].strip()
        if not line:
            continue
        name, _, site = line.partition(",")
        name = name.strip()
        if name and name.lower() not in skip:
            out.append((name, site.strip()))
    return out


async def main(argv=None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--names", type=Path, required=True)
    p.add_argument("--out", type=Path, default=config.REPO_ROOT / "seeds" / "discovered_boards.csv")
    p.add_argument("--seeds", type=Path, default=config.REPO_ROOT / "seeds" / "companies.csv",
                   help="names already in this file are skipped")
    args = p.parse_args(argv)
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(message)s", datefmt="%H:%M:%S")
    logging.getLogger("httpx").setLevel(logging.WARNING)
    settings = config.load()
    skip = {r["name"].lower() for r in csv.DictReader(args.seeds.open())} if args.seeds.exists() else set()
    names = read_names(args.names, skip)
    log.info("probing %d names", len(names))

    slots = asyncio.Semaphore(16)
    async with PoliteClient(settings.user_agent, settings.robots_token, settings.min_gap_seconds,
                            settings.max_gap_seconds, timeout=20, max_attempts=2) as client:
        async def one(name, site):
            async with slots:
                hit = await probe(client, name)
                if hit:
                    log.info("%-28s %-10s %-24s %4d jobs, %d in India", name, hit.ats, hit.slug, hit.jobs, hit.india)
                return name, site, hit

        results = await asyncio.gather(*(one(n, s) for n, s in names))

    found = [(n, s, h) for n, s, h in results if h]
    with args.out.open("w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=FIELDS)
        w.writeheader()
        for name, site, h in found:
            w.writerow({"name": name, "website": site, "careers_url": PUBLIC_URL[h.ats].format(slug=h.slug),
                        "city": h.city, "ats": h.ats,
                        "note": f"probed: {h.india} of {h.jobs} jobs in India; verify city and that the board is this company"})
    log.info("%d of %d names have a board -> %s", len(found), len(names), args.out)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
