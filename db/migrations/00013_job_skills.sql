-- +goose Up
-- Filled by the nightly classifier: skills found in the title and description,
-- and a hash of the text they were read from (re-read only when it changes).
ALTER TABLE jobs ADD COLUMN skills text[] NOT NULL DEFAULT '{}';
ALTER TABLE jobs ADD COLUMN classified_hash text;
CREATE INDEX jobs_skills_idx ON jobs USING GIN (skills);
-- The morning push is sent once a day per user.
ALTER TABLE preferences ADD COLUMN last_push_at timestamptz;

-- +goose Down
ALTER TABLE preferences DROP COLUMN last_push_at;
DROP INDEX jobs_skills_idx;
ALTER TABLE jobs DROP COLUMN classified_hash;
ALTER TABLE jobs DROP COLUMN skills;
