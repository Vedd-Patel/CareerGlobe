-- +goose Up
-- The app is light by default; members can still pick dark or system in S29.
ALTER TABLE preferences ALTER COLUMN theme SET DEFAULT 'light';

-- +goose Down
ALTER TABLE preferences ALTER COLUMN theme SET DEFAULT 'system';
