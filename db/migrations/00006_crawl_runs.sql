-- +goose Up
CREATE TABLE crawl_runs (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    started_at       timestamptz NOT NULL DEFAULT now(),
    -- NULL while running; crawl.py --resume continues the latest unfinished run.
    finished_at      timestamptz,
    companies_ok     integer NOT NULL DEFAULT 0,
    companies_failed integer NOT NULL DEFAULT 0,
    jobs_new         integer NOT NULL DEFAULT 0,
    jobs_closed      integer NOT NULL DEFAULT 0
);

CREATE INDEX crawl_runs_started_at_idx ON crawl_runs (started_at DESC);

-- +goose Down
DROP TABLE crawl_runs;
