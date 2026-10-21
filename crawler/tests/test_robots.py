from robots import Robots

TOKEN = "CareerGlobeBot"

# Keka's real robots.txt (fetched 2026-10-20).
KEKA = """
User-agent: *
Disallow: /
Allow: /careers
Allow: /careers/
"""


def test_keka_longest_match_allows_careers_api():
    r = Robots.parse(KEKA)
    assert r.can_fetch(TOKEN, "https://wingify.keka.com/careers/api/jobs/default/active")
    assert r.can_fetch(TOKEN, "https://wingify.keka.com/careers/")
    assert not r.can_fetch(TOKEN, "https://wingify.keka.com/hr/login")


def test_greenhouse_disallows_only_embed():
    r = Robots.parse("User-agent: *\nDisallow: /embed/\n")
    assert r.can_fetch(TOKEN, "https://boards-api.greenhouse.io/v1/boards/groww/jobs")
    assert not r.can_fetch(TOKEN, "https://boards-api.greenhouse.io/embed/job_board?for=groww")


def test_crawl_delay():
    r = Robots.parse("User-agent: *\nAllow: /\nCrawl-delay: 1\n")
    assert r.crawl_delay(TOKEN) == 1.0
    assert Robots.allow_all().crawl_delay(TOKEN) is None


def test_specific_group_overrides_star():
    r = Robots.parse(
        "User-agent: *\nDisallow: /\n\nUser-agent: careerglobebot\nAllow: /\nDisallow: /private\n"
    )
    assert r.can_fetch(TOKEN, "https://x.com/jobs")
    assert not r.can_fetch(TOKEN, "https://x.com/private/a")
    assert not r.can_fetch("OtherBot", "https://x.com/jobs")


def test_grouped_user_agents_share_rules():
    r = Robots.parse("User-agent: a\nUser-agent: CareerGlobeBot\nDisallow: /x\n")
    assert not r.can_fetch(TOKEN, "https://x.com/x/1")


def test_wildcards_and_anchor():
    r = Robots.parse("User-agent: *\nDisallow: /*.pdf$\nDisallow: /*?session=\n")
    assert not r.can_fetch(TOKEN, "https://x.com/a/b.pdf")
    assert r.can_fetch(TOKEN, "https://x.com/a/b.pdf?x=1")
    assert not r.can_fetch(TOKEN, "https://x.com/jobs?session=1")


def test_allow_wins_tie():
    r = Robots.parse("User-agent: *\nDisallow: /jobs\nAllow: /jobs\n")
    assert r.can_fetch(TOKEN, "https://x.com/jobs")


def test_empty_disallow_and_comments():
    r = Robots.parse("# hello\nUser-agent: * # everyone\nDisallow:\n")
    assert r.can_fetch(TOKEN, "https://x.com/anything")


def test_disallow_all_and_robots_txt_itself():
    r = Robots.disallow_all()
    assert not r.can_fetch(TOKEN, "https://x.com/jobs")
    assert r.can_fetch(TOKEN, "https://x.com/robots.txt")


def test_sitemap_lines_are_kept():
    r = Robots.parse("Sitemap: https://a.com/s1.xml\nUser-agent: *\nDisallow: /x\nsitemap: https://a.com/s2.xml\n")
    assert r.sitemaps == ["https://a.com/s1.xml", "https://a.com/s2.xml"]
    assert not r.can_fetch("Bot", "https://a.com/x")
