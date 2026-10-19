-- +goose Up
CREATE TABLE company_locations (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id      bigint NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    city            text,
    state           text,
    country         text NOT NULL DEFAULT 'India',
    -- Filled by geocoding (Phase 2); NULL until then.
    geo             geography (Point, 4326),
    -- True when the address did not geocode and the city centre was used.
    geo_approximate boolean NOT NULL DEFAULT false,
    is_hq           boolean NOT NULL DEFAULT false,
    UNIQUE NULLS NOT DISTINCT (company_id, city, state, country)
);

CREATE INDEX company_locations_geo_idx ON company_locations USING GIST (geo);

-- +goose Down
DROP TABLE company_locations;
