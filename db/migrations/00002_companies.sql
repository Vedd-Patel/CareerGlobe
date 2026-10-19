-- +goose Up
CREATE TABLE companies (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        text NOT NULL,
    slug        text NOT NULL UNIQUE,
    website     text,
    logo_url    text,
    type        text CHECK (type IN ('product', 'service', 'both')),
    stage       text,
    size_band   text,
    industry    text,
    description text,
    verified_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE companies;
