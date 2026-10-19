-- +goose Up
CREATE TABLE jobs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id    bigint NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    -- sha256 hex of company + ATS job ID (or canonical job URL).
    job_key       text NOT NULL UNIQUE,
    source_job_id text,
    title         text NOT NULL,
    category      text,
    category_2    text,
    seniority     text,
    -- Cleaned HTML, capped at 20,000 characters by the crawler.
    description   text,
    cities        text[] NOT NULL DEFAULT '{}',
    work_mode     text CHECK (work_mode IN ('remote', 'hybrid', 'onsite')),
    -- Annual INR, only when the posting states it.
    salary_min    integer,
    salary_max    integer,
    apply_url     text NOT NULL,
    posted_at     timestamptz,
    first_seen    timestamptz NOT NULL DEFAULT now(),
    last_seen     timestamptz NOT NULL DEFAULT now(),
    -- Consecutive successful crawls of the company without this job; closes at 2.
    missed_runs   smallint NOT NULL DEFAULT 0,
    closed_at     timestamptz,
    status        text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed'))
);

CREATE INDEX jobs_company_status_idx ON jobs (company_id, status);
CREATE INDEX jobs_status_first_seen_idx ON jobs (status, first_seen);

-- +goose Down
DROP TABLE jobs;
