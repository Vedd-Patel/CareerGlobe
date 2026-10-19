import pytest

from urls import canonicalize, job_key


@pytest.mark.parametrize(
    "raw, want",
    [
        # host lowercased, fragment and trailing slash dropped
        ("https://Jobs.Lever.co/Cred/abc/#apply", "https://jobs.lever.co/Cred/abc"),
        # tracking params dropped, the rest sorted
        (
            "https://boards.greenhouse.io/groww/jobs/123?utm_source=x&gh_src=li&b=2&a=1",
            "https://boards.greenhouse.io/groww/jobs/123?a=1&b=2",
        ),
        ("https://x.com/job?ref=home&source=linkedin&UTM_Campaign=c", "https://x.com/job"),
        ("https://x.com/job?lever-source=Careers&lever-origin=applied", "https://x.com/job"),
        # session ids in the query and in the path
        ("https://x.com/job?id=7&JSESSIONID=abc", "https://x.com/job?id=7"),
        ("https://x.com/job;jsessionid=ABC123?id=7", "https://x.com/job?id=7"),
        # default port dropped, other ports kept
        ("https://x.com:443/a", "https://x.com/a"),
        ("http://x.com:8080/a", "http://x.com:8080/a"),
        # root path
        ("https://x.com/", "https://x.com"),
        # path case is kept: paths are case-sensitive on many servers
        ("https://x.com/Jobs/ABC", "https://x.com/Jobs/ABC"),
        # blank values kept
        ("https://x.com/a?q=&p=1", "https://x.com/a?p=1&q="),
    ],
)
def test_canonicalize(raw, want):
    assert canonicalize(raw) == want


def test_canonicalize_is_idempotent():
    url = "https://X.com/a/?utm_medium=e&z=1&a=2#top"
    assert canonicalize(canonicalize(url)) == canonicalize(url)


def test_job_key_ignores_tracking_noise():
    a = job_key(1, None, None, "https://x.com/jobs/9/?utm_source=li")
    b = job_key(1, None, None, "https://X.com/jobs/9#apply")
    assert a == b
    assert len(a) == 64


def test_job_key_prefers_ats_id_over_url():
    a = job_key(1, "lever", "abc", "https://jobs.lever.co/cred/abc")
    b = job_key(1, "lever", "abc", "https://jobs.lever.co/cred/abc?other=1")
    assert a == b


def test_job_key_differs_by_company_and_ats():
    assert job_key(1, "lever", "abc", None) != job_key(2, "lever", "abc", None)
    assert job_key(1, "lever", "abc", None) != job_key(1, "greenhouse", "abc", None)


def test_job_key_needs_id_or_url():
    with pytest.raises(ValueError):
        job_key(1, "lever", None, None)
