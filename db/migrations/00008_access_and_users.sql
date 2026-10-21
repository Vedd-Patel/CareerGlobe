-- +goose Up
-- Invite codes: only HMAC-SHA256(code, server pepper) is stored, never the code.
CREATE TABLE access_codes (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    code_hash   bytea NOT NULL UNIQUE,
    label       text NOT NULL,
    max_uses    integer NOT NULL DEFAULT 1 CHECK (max_uses > 0),
    used_count  integer NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    expires_at  timestamptz,
    revoked     boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (used_count <= max_uses)
);

CREATE TABLE users (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    firebase_uid   text NOT NULL UNIQUE,
    access_code_id bigint NOT NULL REFERENCES access_codes (id),
    name           text,
    email          text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz
);
CREATE INDEX users_access_code_idx ON users (access_code_id);

-- Code guesses, for the 5-per-15-minutes limit and the admin alarm.
CREATE TABLE access_attempts (
    id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ip        inet,
    device_id text,
    at        timestamptz NOT NULL DEFAULT now(),
    success   boolean NOT NULL
);
CREATE INDEX access_attempts_ip_at_idx ON access_attempts (ip, at DESC);
CREATE INDEX access_attempts_device_at_idx ON access_attempts (device_id, at DESC);

-- Verify tickets are single use: a used ticket's ID is remembered until it expires.
CREATE TABLE access_tickets_used (
    ticket_id  text PRIMARY KEY,
    expires_at timestamptz NOT NULL
);

-- Proof of each consent (DPDP).
CREATE TABLE consents (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    policy_version text NOT NULL,
    accepted_at    timestamptz NOT NULL DEFAULT now(),
    ip             inet,
    user_agent     text
);
CREATE INDEX consents_user_idx ON consents (user_id, accepted_at DESC);

-- +goose Down
DROP TABLE consents;
DROP TABLE access_tickets_used;
DROP TABLE access_attempts;
DROP TABLE users;
DROP TABLE access_codes;
