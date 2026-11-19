-- +goose Up
-- When the crawler last looked for a company's exact office position, so a
-- location that has none (only a city centre) is looked up again monthly, not nightly.
ALTER TABLE company_locations ADD COLUMN geo_checked_at timestamptz;

-- +goose Down
ALTER TABLE company_locations DROP COLUMN geo_checked_at;
