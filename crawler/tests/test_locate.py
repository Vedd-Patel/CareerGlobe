"""Office lookups through Overpass: the query, picking the right element, and failure handling."""

import json

import httpx
import pytest

import locate
from fetcher import PoliteClient

BENGALURU = (12.9716, 77.5946)


def el(name, lat, lon, **tags):
    return {"type": "node", "lat": lat, "lon": lon, "tags": {"name": name, **tags}}


def test_query_names_the_company_and_the_area():
    q = locate.query('Dun & "Bradstreet"', *BENGALURU)
    assert '"office"]["name"~"^Dun\\\\ \\\\&\\\\ \\"Bradstreet\\"",i]' in q
    assert "around:25000,12.97160,77.59460" in q and "out center" in q


def test_pick_prefers_offices_then_the_nearest():
    elements = [
        el("Amazon Development Centre", 13.05, 77.7, building="yes"),
        el("Amazon India", 13.10, 77.60, office="company"),
        el("Amazon Pay", 12.99, 77.59, office="company"),
    ]
    assert locate.pick(elements, "Amazon", *BENGALURU) == (12.99, 77.59)


def test_pick_ignores_other_names_and_places_outside_india():
    elements = [el("Amazonas Hospital", 12.98, 77.6, office="x"), el("Amazon", 51.5, -0.12, office="company"),
                {"type": "way", "center": {"lat": 12.98, "lon": 77.6}, "tags": {"name": "Amazon", "office": "company"}}]
    assert locate.pick(elements, "Amazon", *BENGALURU) == (12.98, 77.6)  # the way, via its centre
    assert locate.pick(elements[:2], "Amazon", *BENGALURU) is None


@pytest.mark.anyio
async def test_server_errors_end_the_night_after_three_failures():
    calls = []

    def handler(request):
        calls.append(request.url.path)
        return httpx.Response(404) if request.url.path == "/robots.txt" else httpx.Response(504)

    class Conn:
        async def execute(self, sql, params=None):
            class Cur:
                async def fetchall(self):
                    return [(i, f"Co{i}", 12.9, 77.5) for i in range(10)]
            return Cur()

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, max_attempts=1, transport=httpx.MockTransport(handler)) as c:
        assert await locate.run(Conn(), c, endpoint="https://overpass.test/api/interpreter") == (0, 0)
    assert calls.count("/api/interpreter") == 3  # stopped, did not hammer the server
