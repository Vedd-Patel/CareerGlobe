"""Pins for company locations: city centres from seeds/india_cities.csv.

No network: Nominatim's and GeoNames' robots.txt disallow bots, and seed rows
only give a city anyway, so every pin is the city centre and is marked
geo_approximate (spec: "fall back to the city centre and mark it approximate").
Natural Earth data is public domain; credit it with the map attribution.
"""

import csv
import logging
from pathlib import Path

import config
from clean import CITY_ALIASES

log = logging.getLogger(__name__)
CITIES = config.REPO_ROOT / "seeds" / "india_cities.csv"


def load(path: Path = CITIES) -> dict[str, tuple[float, float]]:
    """Lowercase city name (and its aliases) -> (lat, lon)."""
    with path.open(encoding="utf-8") as f:
        rows = csv.DictReader(line for line in f if not line.startswith("#"))
        coords = {r["city"].strip().lower(): (float(r["lat"]), float(r["lon"])) for r in rows}
    for alias, canonical in CITY_ALIASES.items():
        if canonical.lower() in coords:
            coords.setdefault(alias, coords[canonical.lower()])
    return coords


def lookup(coords: dict[str, tuple[float, float]], city: str | None) -> tuple[float, float] | None:
    return coords.get((city or "").strip().lower()) if city else None


async def add_job_cities(conn, coords: dict[str, tuple[float, float]] | None = None) -> int:
    """Give a company a location for every known city it has open jobs in.

    The seed file names one city per company, but its jobs sit in many. With a
    location per job city the map shows each opening where it is, not all of
    them at the headquarters. Cities that are not in seeds/india_cities.csv
    (a street, a plant name) are skipped: there is no pin to draw. Returns the
    number of locations added; run pin_locations afterwards to give them a pin.
    """
    coords = coords or load()
    cur = await conn.execute(
        "SELECT DISTINCT j.company_id, c FROM jobs j, unnest(j.cities) AS c WHERE j.status = 'open'"
    )
    added = 0
    for company_id, city in await cur.fetchall():
        if lookup(coords, city) is None:
            continue
        res = await conn.execute(
            """INSERT INTO company_locations (company_id, city, is_hq)
               SELECT %(c)s, %(city)s, false
               WHERE NOT EXISTS (SELECT 1 FROM company_locations
                                 WHERE company_id = %(c)s AND lower(city) = lower(%(city)s))""",
            {"c": company_id, "city": city},
        )
        added += res.rowcount
    return added


async def pin_locations(conn, coords: dict[str, tuple[float, float]] | None = None) -> tuple[int, list[str]]:
    """Set geo for every location that has none. Returns (pinned, cities not found)."""
    coords = coords or load()
    cur = await conn.execute("SELECT id, city FROM company_locations WHERE geo IS NULL")
    pinned, missing = 0, set()
    for loc_id, city in await cur.fetchall():
        hit = lookup(coords, city)
        if hit is None:
            missing.add(city or "(no city)")
            continue
        lat, lon = hit
        await conn.execute(
            """UPDATE company_locations
               SET geo = ST_SetSRID(ST_MakePoint(%s, %s), 4326)::geography, geo_approximate = true
               WHERE id = %s""",
            (lon, lat, loc_id),
        )
        pinned += 1
    if missing:
        log.warning("no coordinates for %d cities (add them to seeds/india_cities.csv): %s",
                    len(missing), ", ".join(sorted(missing)))
    return pinned, sorted(missing)
