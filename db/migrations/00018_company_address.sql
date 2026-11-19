-- +goose Up
-- A company office's street address, shown beside its pin (from the seed file).
ALTER TABLE company_locations ADD COLUMN address text;

-- +goose Down
ALTER TABLE company_locations DROP COLUMN address;
