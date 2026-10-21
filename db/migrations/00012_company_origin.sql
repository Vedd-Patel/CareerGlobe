-- +goose Up
-- Where a company came from: the seed file (its source of truth) or the admin
-- console. Seed companies missing from the file stop being crawled.
ALTER TABLE companies ADD COLUMN origin text NOT NULL DEFAULT 'seed' CHECK (origin IN ('seed', 'admin'));

-- +goose Down
ALTER TABLE companies DROP COLUMN origin;
