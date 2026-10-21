-- name: GetProfile :one
SELECT * FROM profiles WHERE user_id = $1;

-- name: UpsertProfile :one
INSERT INTO profiles (user_id, skills, titles, years_exp, current_city, education, confirmed_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(skills)::text[], sqlc.arg(titles)::text[], sqlc.narg(years_exp),
        sqlc.narg(current_city), sqlc.arg(education)::text[],
        CASE WHEN sqlc.arg(confirm)::bool THEN now() END, now())
ON CONFLICT (user_id) DO UPDATE SET
  skills = EXCLUDED.skills, titles = EXCLUDED.titles, years_exp = EXCLUDED.years_exp,
  current_city = EXCLUDED.current_city, education = EXCLUDED.education,
  confirmed_at = COALESCE(EXCLUDED.confirmed_at, profiles.confirmed_at), updated_at = now()
RETURNING *;

-- name: GetResume :one
SELECT * FROM resumes WHERE user_id = $1;

-- name: UpsertResume :one
INSERT INTO resumes (user_id, file_path, original_name, content_type, size_bytes, parsed_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id) DO UPDATE SET
  file_path = EXCLUDED.file_path, original_name = EXCLUDED.original_name, content_type = EXCLUDED.content_type,
  size_bytes = EXCLUDED.size_bytes, uploaded_at = now(), parsed_at = EXCLUDED.parsed_at
RETURNING *;

-- name: RecordConsent :exec
INSERT INTO consents (user_id, policy_version, ip, user_agent) VALUES ($1, $2, $3, $4);

-- name: ListConsents :many
SELECT policy_version, accepted_at FROM consents WHERE user_id = $1 ORDER BY accepted_at DESC;

-- name: GetPreferences :one
SELECT * FROM preferences WHERE user_id = $1;

-- name: UpsertPreferences :one
INSERT INTO preferences (user_id, categories, cities, anywhere, remote_ok, seniority, min_salary, push_time,
                         push_enabled, theme, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (user_id) DO UPDATE SET
  categories = EXCLUDED.categories, cities = EXCLUDED.cities, anywhere = EXCLUDED.anywhere,
  remote_ok = EXCLUDED.remote_ok, seniority = EXCLUDED.seniority, min_salary = EXCLUDED.min_salary,
  push_time = EXCLUDED.push_time, push_enabled = EXCLUDED.push_enabled, theme = EXCLUDED.theme, updated_at = now()
RETURNING *;

-- name: UpsertPushToken :exec
INSERT INTO push_tokens (user_id, platform, fcm_token, updated_at) VALUES ($1, $2, $3, now())
ON CONFLICT (fcm_token) DO UPDATE SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now();

-- name: DeletePushToken :exec
DELETE FROM push_tokens WHERE user_id = $1 AND fcm_token = $2;

-- name: ListNotifications :many
SELECT * FROM notifications WHERE user_id = $1 ORDER BY created_at DESC LIMIT 100;

-- name: MarkNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL
  AND (sqlc.narg(id)::bigint IS NULL OR id = sqlc.narg(id));

-- name: CountUnreadNotifications :one
SELECT count(*)::int FROM notifications WHERE user_id = $1 AND read_at IS NULL;
