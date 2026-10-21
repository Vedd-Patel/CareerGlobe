import pytest

from clean import MAX_DESCRIPTION_CHARS, annual_inr, cities_from, clean_html, work_mode_from


@pytest.mark.parametrize(
    "locations, want",
    [
        (["Bengaluru-VTP, India"], ["Bengaluru"]),
        (["Bangalore"], ["Bengaluru"]),
        (["Gurgaon, Haryana", "New Delhi"], ["Gurugram", "Delhi"]),
        (["Navi Mumbai"], ["Navi Mumbai"]),
        (["Mumbai / Pune"], ["Mumbai", "Pune"]),
        (["India"], []),
        (["multiple"], []),
        (["Remote"], []),
        (["Pan India"], []),
        (["Indore, MP"], ["Indore"]),
        (["Mysore"], ["Mysuru"]),
        (["New York"], ["New York"]),  # unknown cities pass through
        # HackerRank's real labels: the work mode is not part of the city
        (["Hybrid in Santa Clara, CA"], ["Santa Clara"]),
        (["Hybrid in Bangalore, India"], ["Bengaluru"]),
        (["Remote - Kolhapur"], ["Kolhapur"]),
        (["Onsite: Ranchi"], ["Ranchi"]),
        (["Remote Globally"], []),
        (["Remote-US"], []),
        (["Remote, UK"], []),
        (["Bengaluru", "Bangalore, Karnataka"], ["Bengaluru"]),  # no duplicates
        ([""], []),
    ],
)
def test_cities_from(locations, want):
    assert cities_from(locations) == want


def test_work_mode_from():
    assert work_mode_from(["Remote"]) == "remote"
    assert work_mode_from(["Bengaluru (Hybrid)"]) == "hybrid"
    assert work_mode_from(["Work from home"]) == "remote"
    assert work_mode_from(["Bengaluru"]) is None


def test_annual_inr():
    assert annual_inr(1_200_000, 1_800_000, "INR", "year") == (1_200_000, 1_800_000)
    assert annual_inr(50_000, 80_000, "inr", "month") == (600_000, 960_000)
    assert annual_inr(90_000, 120_000, "USD", "year") == (None, None)  # never convert currencies
    assert annual_inr(500, 800, "INR", None) == (None, None)  # unknown period: don't guess
    assert annual_inr(0, 0, "INR", "year") == (None, None)  # Keka sends zeros when hidden
    assert annual_inr(900_000, 600_000, "INR", "year") == (600_000, 900_000)
    # Cars24's real "INR 3,500-4,500 per month" for a Dubai job: implausible, dropped
    assert annual_inr(3500, 4500, "INR", "month") == (None, None)
    assert annual_inr(10_000, 15_000, "INR", "month") == (120_000, 180_000)  # an internship stipend is fine
    assert annual_inr(5_000_000, 2_000_000_000, "INR", "year") == (None, None)


def test_clean_html_strips_scripts_styles_and_attributes():
    raw = (
        '<div style="color:red" onclick="x()"><strong class="a">Hi</strong></div>'
        "<script>alert(1)</script><style>p{}</style>"
        '<a href="javascript:alert(1)">bad</a> <a href="https://x.com/apply" target="_blank">ok</a>'
        "<img src=x onerror=alert(1)><span>text</span>"
    )
    out = clean_html(raw)
    assert out == (
        '<p><strong>Hi</strong></p><a>bad</a> '
        '<a href="https://x.com/apply" rel="nofollow noopener">ok</a>text'
    )


def test_clean_html_escapes_text_and_drops_empty_paragraphs():
    assert clean_html("<p>a &lt; b &amp; c</p><p> </p><p>&nbsp;</p>") == "<p>a &lt; b &amp; c</p>"
    assert clean_html("<p>Tom &amp; Jerry<br/>next</p>") == "<p>Tom &amp; Jerry<br>next</p>"


def test_clean_html_caps_length_without_breaking_a_tag():
    out = clean_html("<p>" + "word &amp; " * 5000 + "</p>")
    assert len(out) <= MAX_DESCRIPTION_CHARS
    assert out.count("<") == out.count(">")
    assert not out.rstrip().endswith("&")


def test_clean_html_empty():
    assert clean_html(None) is None
    assert clean_html("") is None
    assert clean_html("<script>x</script>") is None


def test_clean_html_wrapper_divs_do_not_nest_paragraphs():
    raw = "<div><div><h2>Role</h2></div><div><p>Intro</p></div><div>plain <b>text</b></div><div>more<br></div></div>"
    assert clean_html(raw) == "<h2>Role</h2><p>Intro</p><p>plain <b>text</b></p><p>more</p>"


def test_cities_country_code_and_underscores():
    assert cities_from(["IN"]) == []
    assert cities_from(["Pune_RD, MH, IN"]) == ["Pune"]
