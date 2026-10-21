-- name: ListApplications :many
SELECT a.*, j.title, j.status AS job_status, j.closed_at AS job_closed_at, j.cities, j.work_mode,
       c.id AS company_id, c.name AS company_name, c.logo_url
FROM applications a
JOIN jobs j ON j.id = a.job_id
JOIN companies c ON c.id = j.company_id
WHERE a.user_id = $1
ORDER BY a.updated_at DESC;

-- name: GetApplication :one
SELECT a.*, j.title, j.status AS job_status, j.closed_at AS job_closed_at, j.cities, j.work_mode, j.apply_url,
       c.id AS company_id, c.name AS company_name, c.logo_url
FROM applications a
JOIN jobs j ON j.id = a.job_id
JOIN companies c ON c.id = j.company_id
WHERE a.id = $1 AND a.user_id = $2;

-- name: GetApplicationForJob :one
SELECT * FROM applications WHERE user_id = $1 AND job_id = $2;

-- name: CreateApplication :one
INSERT INTO applications (user_id, job_id, status, applied_at)
VALUES ($1, $2, $3, CASE WHEN $3 = 'applied' THEN now() END)
ON CONFLICT (user_id, job_id) DO UPDATE SET updated_at = now()
RETURNING *;

-- name: UpdateApplication :one
UPDATE applications SET
  status       = sqlc.arg(status)::text,
  applied_at   = CASE WHEN sqlc.arg(status)::text = 'applied' AND applied_at IS NULL THEN now() ELSE applied_at END,
  notes        = sqlc.arg(notes)::text,
  next_step    = sqlc.narg(next_step)::text,
  next_step_at = sqlc.narg(next_step_at)::timestamptz,
  dont_ask     = sqlc.arg(dont_ask)::bool,
  updated_at   = now()
WHERE id = sqlc.arg(id)::bigint AND user_id = sqlc.arg(user_id)::bigint
RETURNING *;

-- name: DeleteApplication :exec
DELETE FROM applications WHERE id = $1 AND user_id = $2;

-- name: AddApplicationEvent :exec
INSERT INTO application_events (application_id, from_status, to_status, note) VALUES ($1, $2, $3, $4);

-- name: ApplicationEvents :many
SELECT * FROM application_events WHERE application_id = $1 ORDER BY at, id;

-- name: AddInterviewRound :one
INSERT INTO interview_rounds (application_id, at, kind, notes) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: InterviewRounds :many
SELECT * FROM interview_rounds WHERE application_id = $1 ORDER BY at, id;

-- name: DeleteInterviewRound :exec
DELETE FROM interview_rounds r USING applications a
WHERE r.id = $1 AND r.application_id = a.id AND a.user_id = $2;

-- name: TrackerStats :one
SELECT count(*) FILTER (WHERE status = 'saved')::int AS saved,
       count(*) FILTER (WHERE status = 'applied')::int AS applied,
       count(*) FILTER (WHERE status = 'interview')::int AS interview,
       count(*) FILTER (WHERE status = 'offer')::int AS offer,
       count(*) FILTER (WHERE status = 'rejected')::int AS rejected,
       count(*) FILTER (WHERE status = 'withdrawn')::int AS withdrawn,
       count(*) FILTER (WHERE applied_at IS NOT NULL)::int AS ever_applied,
       count(*) FILTER (WHERE applied_at IS NOT NULL AND EXISTS (
         SELECT 1 FROM application_events e WHERE e.application_id = applications.id AND e.to_status = 'interview'))::int AS reached_interview
FROM applications WHERE user_id = $1;
