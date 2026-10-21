-- +goose Up
-- Data-removal and "remove my company" requests (handled from the admin console).
CREATE TABLE removal_requests (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        text NOT NULL DEFAULT 'user_data' CHECK (kind IN ('user_data', 'company')),
    email       text NOT NULL,
    user_id     bigint REFERENCES users (id) ON DELETE SET NULL,
    company_id  bigint REFERENCES companies (id) ON DELETE SET NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    done_at     timestamptz,
    note        text NOT NULL DEFAULT ''
);

-- Title keyword -> role category; editable in admin without a deploy.
CREATE TABLE category_rules (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    keyword  text NOT NULL UNIQUE,
    category text NOT NULL,
    priority integer NOT NULL DEFAULT 100
);

-- Skill spelling -> canonical skill (JS -> JavaScript), applied to resumes and jobs.
CREATE TABLE skill_synonyms (
    alias     text PRIMARY KEY,
    canonical text NOT NULL
);

CREATE TABLE city_aliases (
    alias     text PRIMARY KEY,
    canonical text NOT NULL,
    state     text
);

-- Job reports from the "Report wrong info" link (S18).
CREATE TABLE reports (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint REFERENCES users (id) ON DELETE SET NULL,
    company_id bigint REFERENCES companies (id) ON DELETE CASCADE,
    job_id     bigint REFERENCES jobs (id) ON DELETE CASCADE,
    message    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    done_at    timestamptz
);

-- The GitHub watchdog asks for a re-run when last night's crawl did not finish;
-- the server's trigger timer picks pending rows up.
CREATE TABLE crawl_triggers (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    requested_at timestamptz NOT NULL DEFAULT now(),
    reason       text NOT NULL,
    started_at   timestamptz
);

-- Jobs: classification filled after each crawl.
CREATE INDEX jobs_category_idx ON jobs (category) WHERE status = 'open';
CREATE INDEX jobs_cities_idx ON jobs USING GIN (cities);

-- +goose Down
DROP INDEX jobs_cities_idx;
DROP INDEX jobs_category_idx;
DROP TABLE crawl_triggers;
DROP TABLE reports;
DROP TABLE city_aliases;
DROP TABLE skill_synonyms;
DROP TABLE category_rules;
DROP TABLE removal_requests;
