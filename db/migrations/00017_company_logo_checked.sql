-- +goose Up
-- When the crawler last looked for a company's logo, so a company whose site has
-- none is tried again monthly, not nightly.
ALTER TABLE companies ADD COLUMN logo_checked_at timestamptz;

-- +goose Down
ALTER TABLE companies DROP COLUMN logo_checked_at;
