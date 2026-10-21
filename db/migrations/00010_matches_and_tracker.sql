-- +goose Up
-- Rebuilt nightly by the matcher.
CREATE TABLE matches (
    user_id     bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id      bigint NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    score       smallint NOT NULL CHECK (score BETWEEN 0 AND 100),
    reasons     jsonb NOT NULL DEFAULT '{}',
    computed_at timestamptz NOT NULL DEFAULT now(),
    notified_at timestamptz,
    seen        boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, job_id)
);
CREATE INDEX matches_user_score_idx ON matches (user_id, score DESC);

CREATE TABLE applications (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id       bigint NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'saved'
                 CHECK (status IN ('saved', 'applied', 'interview', 'offer', 'rejected', 'withdrawn')),
    applied_at   timestamptz,
    notes        text NOT NULL DEFAULT '',
    next_step    text,
    next_step_at timestamptz,
    -- "Don't ask for this job" on the did-you-apply prompt (S21).
    dont_ask     boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, job_id)
);
CREATE INDEX applications_user_status_idx ON applications (user_id, status);

CREATE TABLE application_events (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id bigint NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    from_status    text,
    to_status      text NOT NULL,
    at             timestamptz NOT NULL DEFAULT now(),
    note           text
);
CREATE INDEX application_events_app_idx ON application_events (application_id, at);

-- Interview rounds on an application (S24).
CREATE TABLE interview_rounds (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id bigint NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    at             timestamptz NOT NULL,
    kind           text NOT NULL,
    notes          text NOT NULL DEFAULT ''
);
CREATE INDEX interview_rounds_app_idx ON interview_rounds (application_id, at);

CREATE TABLE saved_companies (
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    company_id bigint NOT NULL REFERENCES companies (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_id)
);

-- Notifications inbox (S27): morning matches, saved-company posts, reminders.
CREATE TABLE notifications (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('matches', 'saved_company', 'reminder', 'system')),
    title      text NOT NULL,
    body       text NOT NULL DEFAULT '',
    link       text,
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at    timestamptz
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);

-- +goose Down
DROP TABLE notifications;
DROP TABLE saved_companies;
DROP TABLE interview_rounds;
DROP TABLE application_events;
DROP TABLE applications;
DROP TABLE matches;
