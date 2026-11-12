-- +goose Up
-- 'system' was only ever the old column default (the app had no users when the
-- default became light), so rows that still carry it move to light as well.
UPDATE preferences SET theme = 'light' WHERE theme = 'system';

-- +goose Down
SELECT 1; -- the earlier values cannot be told apart from chosen ones
