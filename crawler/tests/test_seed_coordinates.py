"""Optional lat/lon columns in the seed file."""

import logging

import seeds

HEADER = "name,website,careers_url,city,state,type,ats,lat,lon\n"


def read(tmp_path, *rows):
    path = tmp_path / "companies.csv"
    path.write_text(HEADER + "\n".join(rows) + "\n", encoding="utf-8")
    return seeds.read(path)


def test_coordinates_are_read_when_present(tmp_path):
    rows = read(tmp_path, "Acme,https://acme.in,https://boards.greenhouse.io/acme,Pune,Maharashtra,product,,18.5590,73.7868")
    assert (rows[0].lat, rows[0].lon) == (18.559, 73.7868)


def test_missing_coordinates_leave_the_pin_approximate(tmp_path):
    rows = read(tmp_path, "Acme,https://acme.in,https://boards.greenhouse.io/acme,Pune,Maharashtra,product,,,")
    assert (rows[0].lat, rows[0].lon) == (None, None)


def test_bad_or_foreign_coordinates_are_ignored_with_a_warning(tmp_path, caplog):
    with caplog.at_level(logging.WARNING):
        rows = read(
            tmp_path,
            "A,https://a.in,https://boards.greenhouse.io/a,Pune,Maharashtra,product,,18.5,north",
            "B,https://b.in,https://boards.greenhouse.io/b,Pune,Maharashtra,product,,51.5,-0.12",
            "C,https://c.in,https://boards.greenhouse.io/c,Pune,Maharashtra,product,,18.5,",
        )
    assert [(r.lat, r.lon) for r in rows] == [(None, None)] * 3
    assert caplog.text.count("left approximate") == 3


def test_seed_file_without_the_columns_still_loads(tmp_path):
    path = tmp_path / "companies.csv"
    path.write_text("name,website,careers_url,city,state,type,ats\nAcme,,https://boards.greenhouse.io/acme,Pune,Maharashtra,product,\n")
    assert seeds.read(path)[0].lat is None


def test_address_column_is_read_and_trimmed(tmp_path):
    path = tmp_path / "companies.csv"
    path.write_text("name,website,careers_url,city,state,type,ats,address\n"
                    "Acme,,https://boards.greenhouse.io/acme,Pune,Maharashtra,product,,\"  Plot 4, Kharadi, Pune 411014 \"\n"
                    "Beta,,https://boards.greenhouse.io/beta,Pune,Maharashtra,product,,\n", encoding="utf-8")
    rows = seeds.read(path)
    assert rows[0].address == "Plot 4, Kharadi, Pune 411014"
    assert rows[1].address is None
