"""Load seeds/companies.csv into Postgres (companies, locations, careers sources).

Columns: name,website,careers_url,city,state,type,ats[,note][,lat,lon][,address]
lat and lon are optional: the office's exact position (decimal degrees, inside
India). With them the map points at the office itself; without them the pin is
the city centre and is marked approximate. address is the office's street address,
shown beside its pin.
careers_url is an ATS board (detected from the URL), a SuccessFactors site
(ats=successfactors), or any other careers page, read for JSON-LD JobPostings.

The file is the source of truth for seed companies: a company (or careers URL)
removed from it stops being crawled, and ats=do-not-crawl honours a company's
opt-out. Companies added in the admin console (origin 'admin') are left alone.
"""

import csv
import logging
import re
from dataclasses import dataclass
from pathlib import Path

import psycopg

from adapters import detect, source_for

log = logging.getLogger(__name__)
TYPES = {"product", "service", "both"}
DO_NOT_CRAWL = "do-not-crawl"


@dataclass
class SeedRow:
    name: str
    slug: str
    website: str | None
    careers_url: str
    city: str | None
    state: str | None
    type: str | None
    ats: str | None
    ats_slug: str | None
    do_not_crawl: bool = False
    lat: float | None = None
    lon: float | None = None
    address: str | None = None


INDIA_BOX = (6.0, 38.0, 67.0, 98.0)  # south, north, west, east


def coordinates(row: dict, line: int) -> tuple[float | None, float | None]:
    """The optional lat/lon columns; both must be present, numeric and inside India."""
    raw_lat, raw_lon = (row.get("lat") or "").strip(), (row.get("lon") or "").strip()
    if not raw_lat and not raw_lon:
        return None, None
    try:
        lat, lon = float(raw_lat), float(raw_lon)
    except ValueError:
        log.warning("seeds line %d: lat and lon must both be numbers; location left approximate", line)
        return None, None
    south, north, west, east = INDIA_BOX
    if not (south <= lat <= north and west <= lon <= east):
        log.warning("seeds line %d: %s,%s is outside India; location left approximate", line, lat, lon)
        return None, None
    return lat, lon


def slugify(name: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")


def read(path: Path) -> list[SeedRow]:
    rows, seen = [], set()
    with path.open(newline="", encoding="utf-8") as f:
        for n, r in enumerate(csv.DictReader(f), start=2):
            name = (r.get("name") or "").strip()
            url = (r.get("careers_url") or "").strip()
            if not name or not url:
                log.warning("seeds line %d: name and careers_url are required; skipped", n)
                continue
            slug = slugify(name)
            if slug in seen:
                log.warning("seeds line %d: duplicate company %r; skipped", n, name)
                continue
            seen.add(slug)

            ctype = (r.get("type") or "").strip().lower() or None
            if ctype and ctype not in TYPES:
                log.warning("seeds line %d: type %r is not product/service/both; left empty", n, ctype)
                ctype = None

            given = (r.get("ats") or "").strip().lower() or None
            detected = detect(url)
            if detected and given and given != detected[0]:
                log.warning("seeds line %d: ats %r but URL is %s; using %s", n, given, detected[0], detected[0])
            do_not_crawl = given == DO_NOT_CRAWL
            ats, ats_slug = (None, None) if do_not_crawl else source_for(url, given)

            lat, lon = coordinates(r, n)
            rows.append(SeedRow(
                name=name, slug=slug, website=(r.get("website") or "").strip() or None,
                careers_url=url, city=(r.get("city") or "").strip() or None,
                state=(r.get("state") or "").strip() or None, type=ctype, ats=ats, ats_slug=ats_slug,
                do_not_crawl=do_not_crawl, lat=lat, lon=lon,
                address=(r.get("address") or "").strip()[:300] or None,
            ))
    return rows


async def load(conn: psycopg.AsyncConnection, path: Path) -> int:
    rows = read(path)
    kept: list[int] = []
    async with conn.transaction():
        for s in rows:
            cur = await conn.execute(
                """INSERT INTO companies (name, slug, website, type) VALUES (%s, %s, %s, %s)
                   ON CONFLICT (slug) DO UPDATE
                   SET name = EXCLUDED.name, website = EXCLUDED.website,
                       type = COALESCE(EXCLUDED.type, companies.type)
                   RETURNING id""",
                (s.name, s.slug, s.website, s.type),
            )
            company_id = (await cur.fetchone())[0]
            if s.city or s.state:
                await conn.execute(
                    """INSERT INTO company_locations (company_id, city, state, is_hq)
                       VALUES (%s, %s, %s, true) ON CONFLICT DO NOTHING""",
                    (company_id, s.city, s.state),
                )
                if s.address:
                    await conn.execute(
                        """UPDATE company_locations SET address = %s
                           WHERE company_id = %s AND city IS NOT DISTINCT FROM %s AND state IS NOT DISTINCT FROM %s""",
                        (s.address, company_id, s.city, s.state),
                    )
                if s.lat is not None and s.lon is not None:
                    # An exact position from the seed file replaces the city-centre pin.
                    await conn.execute(
                        """UPDATE company_locations
                           SET geo = ST_SetSRID(ST_MakePoint(%s, %s), 4326)::geography, geo_approximate = false
                           WHERE company_id = %s AND city IS NOT DISTINCT FROM %s AND state IS NOT DISTINCT FROM %s""",
                        (s.lon, s.lat, company_id, s.city, s.state),
                    )
            cur = await conn.execute(
                """INSERT INTO careers_sources (company_id, careers_url, ats_type, ats_slug, status)
                   VALUES (%(c)s, %(u)s, %(a)s, %(s)s, CASE WHEN %(off)s THEN 'disabled' ELSE 'active' END)
                   ON CONFLICT (company_id, careers_url) DO UPDATE
                   SET ats_type = EXCLUDED.ats_type, ats_slug = EXCLUDED.ats_slug,
                       status = CASE WHEN %(off)s THEN 'disabled'
                                     WHEN careers_sources.status = 'disabled' THEN 'active'
                                     ELSE careers_sources.status END
                   RETURNING id""",
                {"c": company_id, "u": s.careers_url, "a": s.ats, "s": s.ats_slug, "off": s.do_not_crawl},
            )
            kept.append((await cur.fetchone())[0])
        # Seed companies (or old careers URLs) no longer in the file stop being crawled.
        cur = await conn.execute(
            """UPDATE careers_sources s SET status = 'disabled'
               FROM companies c
               WHERE c.id = s.company_id AND c.origin = 'seed' AND s.status <> 'disabled'
                 AND NOT (s.id = ANY(%s))""",
            (kept,),
        )
        if cur.rowcount:
            log.info("disabled %d careers sources no longer in %s", cur.rowcount, path.name)
    return len(rows)
