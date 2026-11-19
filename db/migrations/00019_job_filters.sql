-- +goose Up
-- What kind of job it is and how many years it asks for, read from the posting by the
-- nightly classifier, so jobs can be filtered by type (intern, full time...) and experience.
ALTER TABLE jobs ADD COLUMN employment_type text
    CHECK (employment_type IN ('intern', 'full_time', 'part_time', 'contract'));
ALTER TABLE jobs ADD COLUMN years_min smallint;
ALTER TABLE jobs ADD COLUMN years_max smallint;  -- NULL with years_min set means "N or more"
CREATE INDEX jobs_employment_idx ON jobs (employment_type) WHERE status = 'open';
-- Read every open posting again with the new rules.
UPDATE jobs SET classified_hash = NULL WHERE status = 'open';

-- +goose Down
DROP INDEX jobs_employment_idx;
ALTER TABLE jobs DROP COLUMN years_max;
ALTER TABLE jobs DROP COLUMN years_min;
ALTER TABLE jobs DROP COLUMN employment_type;
