"""Exact office positions from OpenStreetMap, read through an Overpass server.

A location starts as a city centre (geocode.py). For each such location this
asks Overpass for an office or building near that city whose name starts with
the company's name, and moves the pin there when one is mapped. Companies that
are not mapped keep the city-centre pin.

Overpass is an API for programs. overpass-api.de disallows /api/ for bots in its
robots.txt, so it is not used; the default is a community server with no
robots.txt, and OVERPASS_URL points it elsewhere (also a self-hosted one). It
goes through the polite client: robots.txt, one request at a time, 1-2 s apart,
an honest user agent. Everything here is best effort: an unreachable server
skips the step and is tried again the next night. Data: © OpenStreetMap
contributors (ODbL), credited by the map's attribution.
"""

import json
import logging
import os
import re

from fetcher import FetchError
from urllib.parse import urlencode

log = logging.getLogger(__name__)
OVERPASS = os.environ.get("OVERPASS_URL") or "https://overpass.private.coffee/api/interpreter"
RADIUS_M = 25_000
RECHECK_DAYS = 30
PER_RUN = 60
MAX_FAILS = 3
INDIA_BOX = (6.0, 38.0, 67.0, 98.0)  # south, north, west, east


def norm(text: str) -> str:
    return " ".join(re.findall(r"[a-z0-9]+", text.lower()))


def query(name: str, lat: float, lon: float) -> str:
    """Overpass QL: offices and named buildings near (lat, lon) whose name starts with `name`."""
    pattern = re.escape(name).replace("\\", "\\\\").replace('"', '\\"')
    around = f"(around:{RADIUS_M},{lat:.5f},{lon:.5f})"
    return (f'[out:json][timeout:20];(nwr["office"]["name"~"^{pattern}",i]{around};'
            f'nwr["building"]["name"~"^{pattern}",i]{around};);out center 20 tags;')


def pick(elements: list[dict], name: str, lat: float, lon: float) -> tuple[float, float] | None:
    """The best mapped office for the company: offices before buildings, then nearest the city centre."""
    want = norm(name)
    south, north, west, east = INDIA_BOX
    best, best_rank = None, None
    for e in elements:
        tags = e.get("tags") or {}
        got = norm(tags.get("name") or "")
        if not got or not (got == want or got.startswith(want + " ")):
            continue
        c = e.get("center") or e
        elat, elon = c.get("lat"), c.get("lon")
        if not isinstance(elat, (int, float)) or not isinstance(elon, (int, float)):
            continue
        if not (south <= elat <= north and west <= elon <= east):
            continue
        rank = (0 if tags.get("office") else 1, (elat - lat) ** 2 + (elon - lon) ** 2)
        if best_rank is None or rank < best_rank:
            best, best_rank = (elat, elon), rank
    return best


async def run(conn, client, limit: int = PER_RUN, endpoint: str | None = None) -> tuple[int, int]:
    """Look up to `limit` city-centre locations. Returns (looked up, moved to an exact office)."""
    endpoint = endpoint or OVERPASS
    cur = await conn.execute(
        """SELECT l.id, c.name, ST_Y(l.geo::geometry), ST_X(l.geo::geometry)
           FROM company_locations l JOIN companies c ON c.id = l.company_id
           WHERE l.geo IS NOT NULL AND l.geo_approximate
             AND (l.geo_checked_at IS NULL OR l.geo_checked_at < now() - make_interval(days => %s))
           ORDER BY l.is_hq DESC, l.id LIMIT %s""",
        (RECHECK_DAYS, limit),
    )
    todo = await cur.fetchall()
    checked = found = fails = 0
    for loc_id, name, lat, lon in todo:
        try:
            res = await client.get(f"{endpoint}?{urlencode({'data': query(name, lat, lon)})}")
            elements = json.loads(res.content.decode("utf-8", "replace")).get("elements") or []
        except (FetchError, ValueError, AttributeError) as e:
            fails += 1
            log.warning("office lookup for %r failed (%s)", name, e)
            if fails >= MAX_FAILS:
                log.warning("giving up on office lookups for tonight after %d failures", fails)
                break
            continue
        fails = 0
        checked += 1
        hit = pick(elements, name, lat, lon)
        if hit:
            found += 1
            await conn.execute(
                """UPDATE company_locations
                   SET geo = ST_SetSRID(ST_MakePoint(%s, %s), 4326)::geography, geo_approximate = false,
                       geo_checked_at = now() WHERE id = %s""",
                (hit[1], hit[0], loc_id),
            )
        else:
            await conn.execute("UPDATE company_locations SET geo_checked_at = now() WHERE id = %s", (loc_id,))
    return checked, found
