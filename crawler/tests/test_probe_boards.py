import json

import httpx
import pytest

import probe_boards
from fetcher import PoliteClient


def test_slugs():
    assert probe_boards.slugs("Urban Company") == ["urbancompany", "urban-company"]
    assert probe_boards.slugs("Zeta Technologies Pvt Ltd")[:1] == ["zeta"]
    assert probe_boards.slugs("CRED") == ["cred"]


def board(jobs):
    return {"jobs": [{"id": i, "title": f"Job {i}", "absolute_url": f"https://x/{i}", "location": {"name": loc}}
                     for i, loc in enumerate(jobs)]}


@pytest.mark.anyio
async def test_probe_keeps_india_boards_and_rejects_foreign_namesakes():
    boards = {"/v1/boards/acme/jobs": board(["Bengaluru, India", "Pune", "London, UK"]),
              "/v1/boards/globex/jobs": board(["New York, United States", "London, UK"])}

    def handler(request):
        if request.url.path == "/robots.txt":
            return httpx.Response(404)
        if request.url.path in boards:
            return httpx.Response(200, json=boards[request.url.path])
        return httpx.Response(404)

    async with PoliteClient("Bot/1", "Bot", 0, 0, backoff=0, max_attempts=1, transport=httpx.MockTransport(handler)) as c:
        hit = await probe_boards.probe(c, "Acme")
        assert (hit.ats, hit.slug, hit.jobs, hit.india, hit.city) == ("greenhouse", "acme", 3, 2, "Bengaluru")
        assert await probe_boards.probe(c, "Globex") is None
        assert await probe_boards.probe(c, "Nobody") is None
