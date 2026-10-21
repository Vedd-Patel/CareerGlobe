-- +goose Up
-- The source's own "last changed" stamp for a job (e.g. a sitemap <lastmod>),
-- so a job page is fetched again only when it changed.
ALTER TABLE jobs ADD COLUMN source_updated text;

-- +goose Down
ALTER TABLE jobs DROP COLUMN source_updated;
