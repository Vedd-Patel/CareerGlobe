"""Adapters against saved real responses (trimmed), fetched 2026-10-20."""

import json
from datetime import datetime, timezone
from pathlib import Path

import pytest

from adapters import ADAPTERS, ParseError, detect

FIXTURES = Path(__file__).parent / "fixtures"


def load(name):
    return json.loads((FIXTURES / name).read_text())


def test_greenhouse_groww():
    jobs = ADAPTERS["greenhouse"].parse(load("greenhouse_groww.json"), "groww")
    assert [j.source_job_id for j in jobs] == ["4970739101", "5000706101", "4528370101"]

    j = jobs[0]
    assert j.title == "Assistant Manager - Internal Audit"
    assert j.apply_url == "https://job-boards.eu.greenhouse.io/groww/jobs/4970739101"
    assert j.cities == ["Bengaluru"]  # from "Bengaluru-VTP, India"
    assert j.work_mode is None
    assert j.salary_min is None and j.salary_max is None
    assert j.posted_at == datetime.fromisoformat("2026-09-14T03:49:28-04:00")
    # content arrives entity-escaped; it must come out as real, clean HTML
    assert j.description.startswith("<p><strong>About Groww:</strong></p>")
    assert "&lt;" not in j.description[:200]

    assert jobs[2].cities == []  # location "India": no city is guessed


def test_lever_cred():
    jobs = ADAPTERS["lever"].parse(load("lever_cred.json"), "cred")
    assert len(jobs) == 3

    j = jobs[0]
    assert j.source_job_id == "aa98a938-af3a-45f5-851d-966b2b5bad33"
    assert j.title == "accounts payable manager"
    assert j.apply_url == "https://jobs.lever.co/cred/aa98a938-af3a-45f5-851d-966b2b5bad33"
    assert j.cities == ["Hyderabad"]
    assert j.work_mode == "onsite"
    assert j.posted_at == datetime(2026, 9, 28, 5, 39, 42, 21000, tzinfo=timezone.utc)
    assert "<h3></h3>" not in j.description

    assert jobs[1].cities == []  # "multiple"
    assert "<h3>What will you do:</h3>" in jobs[1].description


def test_keka_wingify():
    jobs = ADAPTERS["keka"].parse(load("keka_wingify.json"), "wingify")
    assert [j.source_job_id for j in jobs] == ["144029", "143899", "143827", "143142"]

    j = jobs[0]
    assert j.title == "Product Marketing Intern"
    assert j.apply_url == "https://wingify.keka.com/careers/jobdetails/144029"
    assert j.cities == ["Delhi"]
    assert j.work_mode == "remote"  # location label "Remote"
    assert j.salary_min is None  # salaryPeriod 0: not available
    assert j.posted_at == datetime(2026, 10, 9, 13, 23, 58, 727000, tzinfo=timezone.utc)
    assert 'style="' not in j.description

    assert jobs[2].cities == ["New York"]
    assert jobs[2].work_mode is None


def test_keka_salary_when_stated():
    data = [{"id": 1, "title": "SDE", "jobLocations": [],
             "salaryRange": {"currency": "INR", "salaryPeriod": 3, "minimum": 50000, "maximum": 70000}}]
    j = ADAPTERS["keka"].parse(data, "acme")[0]
    assert (j.salary_min, j.salary_max) == (600_000, 840_000)


def test_lever_salary_and_unspecified_workplace():
    data = [{"id": "x", "text": "SDE", "hostedUrl": "https://jobs.lever.co/a/x", "workplaceType": "unspecified",
             "categories": {"allLocations": ["Bengaluru (Hybrid)"]},
             "salaryRange": {"currency": "INR", "interval": "per-year-salary", "min": 2000000, "max": 3000000}}]
    j = ADAPTERS["lever"].parse(data, "a")[0]
    assert j.work_mode == "hybrid"
    assert (j.salary_min, j.salary_max) == (2_000_000, 3_000_000)


def test_greenhouse_office_names_are_not_cities():
    data = {"jobs": [{"id": 1, "title": "Analyst", "absolute_url": "https://x.com/1",
                      "location": {"name": "Bengaluru"},
                      "offices": [{"name": "Payments", "location": None}]}]}
    assert ADAPTERS["greenhouse"].parse(data, "razorpay")[0].cities == ["Bengaluru"]


def test_jobs_missing_required_fields_are_skipped():
    assert ADAPTERS["greenhouse"].parse({"jobs": [{"id": 1, "title": "x"}]}, "a") == []
    assert ADAPTERS["lever"].parse([{"id": "x", "text": "y"}], "a") == []
    assert ADAPTERS["keka"].parse([{"title": "no id"}], "a") == []


@pytest.mark.parametrize(
    "ats, bad",
    [("greenhouse", []), ("greenhouse", {"error": "Job not found"}), ("lever", {"ok": False}), ("keka", {})],
)
def test_unexpected_shape_raises(ats, bad):
    with pytest.raises(ParseError):
        ADAPTERS[ats].parse(bad, "a")


def test_empty_boards_parse_to_no_jobs():
    assert ADAPTERS["greenhouse"].parse({"jobs": [], "meta": {"total": 0}}, "a") == []
    assert ADAPTERS["lever"].parse([], "a") == []
    assert ADAPTERS["keka"].parse([], "a") == []


@pytest.mark.parametrize(
    "url, want",
    [
        ("https://boards.greenhouse.io/inmobi", ("greenhouse", "inmobi")),
        ("https://job-boards.eu.greenhouse.io/groww/jobs/123", ("greenhouse", "groww")),
        ("https://boards.greenhouse.io/embed/job_board?for=druva", ("greenhouse", "druva")),
        ("https://boards-api.greenhouse.io/v1/boards/razorpaysoftwareprivatelimited/jobs",
         ("greenhouse", "razorpaysoftwareprivatelimited")),
        ("https://jobs.lever.co/cred", ("lever", "cred")),
        ("https://jobs.lever.co/meesho/abc-123", ("lever", "meesho")),
        ("https://jobs.eu.lever.co/acme", ("lever_eu", "acme")),
        ("https://wingify.keka.com/careers/", ("keka", "wingify")),
        ("https://Toddle.Keka.com/careers", ("keka", "toddle")),
        ("https://www.keka.com/", None),
        ("https://nvidia.wd5.myworkdayjobs.com/NVIDIAExternalCareerSite",
         ("workday", "nvidia.wd5/NVIDIAExternalCareerSite")),
        ("https://Adobe.wd5.myworkdayjobs.com/en-US/external_experienced/job/Noida/x_R1",
         ("workday", "adobe.wd5/external_experienced")),
        ("https://salesforce.wd12.myworkdayjobs.com/External_Career_Site/",
         ("workday", "salesforce.wd12/External_Career_Site")),
        ("https://nvidia.wd5.myworkdayjobs.com/", None),
        ("https://careers.zohocorp.com", None),
        ("https://boards.greenhouse.io/embed/job_board", None),
    ],
)
def test_detect(url, want):
    assert detect(url) == want


def test_board_urls_match_the_endpoints_checked_by_hand():
    assert ADAPTERS["greenhouse"].board_url("groww") == "https://boards-api.greenhouse.io/v1/boards/groww/jobs?content=true"
    assert ADAPTERS["lever"].board_url("cred") == "https://api.lever.co/v0/postings/cred?mode=json"
    assert ADAPTERS["lever_eu"].board_url("x") == "https://api.eu.lever.co/v0/postings/x?mode=json"
    assert ADAPTERS["keka"].board_url("wingify") == "https://wingify.keka.com/careers/api/jobs/default/active"
