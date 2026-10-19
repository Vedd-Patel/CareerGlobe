"""Load seeds/companies.csv into Postgres (companies, locations, careers sources).

Columns: name,website,careers_url,city,state,type,ats[,note]
In Phase 1 careers_url must be the ATS board itself (e.g. https://jobs.lever.co/cred);
discover.py (Phase 2) will handle custom careers pages that link to an ATS.
"""

import csv
import logging
import re
from dataclasses import dataclass
from pathlib import Path

import psycopg

from adapters import detect

log = logging.getLogger(__name__)
TYPES = {"product", "service", "both"}


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

            detected = detect(url)
            ats = (r.get("ats") or "").strip().lower() or None
            ats_slug = None
            if detected:
                if ats and ats != detected[0]:
                    log.warning("seeds line %d: ats %r but URL is %s; using %s", n, ats, detected[0], detected[0])
                ats, ats_slug = detected

            rows.append(SeedRow(
                name=name, slug=slug, website=(r.get("website") or "").strip() or None,
                careers_url=url, city=(r.get("city") or "").strip() or None,
                state=(r.get("state") or "").strip() or None, type=ctype, ats=ats, ats_slug=ats_slug,
            ))
    return rows


async def load(conn: psycopg.AsyncConnection, path: Path) -> int:
    rows = read(path)
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
            await conn.execute(
                """INSERT INTO careers_sources (company_id, careers_url, ats_type, ats_slug)
                   VALUES (%s, %s, %s, %s)
                   ON CONFLICT (company_id, careers_url) DO UPDATE
                   SET ats_type = COALESCE(EXCLUDED.ats_type, careers_sources.ats_type),
                       ats_slug = COALESCE(EXCLUDED.ats_slug, careers_sources.ats_slug)""",
                (company_id, s.careers_url, s.ats, s.ats_slug),
            )
    return len(rows)
