-- +goose Up
CREATE TABLE profiles (
    user_id      bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    skills       text[] NOT NULL DEFAULT '{}',
    titles       text[] NOT NULL DEFAULT '{}',
    years_exp    numeric(4, 1),
    current_city text,
    education    text[] NOT NULL DEFAULT '{}',
    confirmed_at timestamptz,
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX profiles_skills_idx ON profiles USING GIN (skills);

-- The uploaded file, AES-256-GCM encrypted, stored outside the web root under a random name.
CREATE TABLE resumes (
    user_id       bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    file_path     text NOT NULL,
    original_name text,
    content_type  text NOT NULL,
    size_bytes    integer NOT NULL,
    uploaded_at   timestamptz NOT NULL DEFAULT now(),
    parsed_at     timestamptz
);

CREATE TABLE preferences (
    user_id     bigint PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    categories  text[] NOT NULL DEFAULT '{}',
    cities      text[] NOT NULL DEFAULT '{}',
    anywhere    boolean NOT NULL DEFAULT false,
    remote_ok   boolean NOT NULL DEFAULT true,
    seniority   text[] NOT NULL DEFAULT '{}',
    -- Annual INR; filters only jobs that state a salary.
    min_salary  integer,
    push_time   time NOT NULL DEFAULT '08:00',
    push_enabled boolean NOT NULL DEFAULT true,
    theme       text NOT NULL DEFAULT 'system' CHECK (theme IN ('system', 'light', 'dark')),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE push_tokens (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform   text NOT NULL CHECK (platform IN ('android', 'ios', 'web')),
    fcm_token  text NOT NULL UNIQUE,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX push_tokens_user_idx ON push_tokens (user_id);

-- +goose Down
DROP TABLE push_tokens;
DROP TABLE preferences;
DROP TABLE resumes;
DROP TABLE profiles;
