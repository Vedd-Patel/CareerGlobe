-- +goose Up
CREATE TABLE careers_sources (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id      bigint NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    careers_url     text NOT NULL,
    -- greenhouse, lever, keka, ... ; NULL until detected.
    ats_type        text,
    ats_slug        text,
    -- CSS selectors for custom pages (title, location, link).
    selectors       jsonb,
    etag            text,
    last_modified   text,
    last_attempt_at timestamptz,
    last_ok_at      timestamptz,
    last_error      text,
    fail_count      integer NOT NULL DEFAULT 0,
    -- broken: sanity check tripped; blocked: robots.txt disallows; review: 3+ failures.
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'broken', 'blocked', 'review', 'disabled')),
    UNIQUE (company_id, careers_url)
);

-- +goose Down
DROP TABLE careers_sources;
