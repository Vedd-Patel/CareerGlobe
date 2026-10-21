-- name: GetAccountByUID :one
-- The per-request check: the user and whether their code still lets them in.
SELECT u.id, u.firebase_uid, u.name, u.email, u.created_at, u.last_login_at,
       c.revoked AS code_revoked
FROM users u JOIN access_codes c ON c.id = u.access_code_id
WHERE u.firebase_uid = $1;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: CreateUser :one
INSERT INTO users (firebase_uid, access_code_id, name, email, last_login_at)
VALUES ($1, $2, $3, $4, now())
RETURNING *;

-- name: TouchLogin :exec
UPDATE users SET last_login_at = now() WHERE id = $1 AND (last_login_at IS NULL OR last_login_at < now() - interval '1 hour');

-- name: UpdateUserName :exec
UPDATE users SET name = $2 WHERE id = $1;

-- name: UsersByCode :many
SELECT id, name, email, created_at, last_login_at FROM users WHERE access_code_id = $1 ORDER BY created_at;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: OnboardingState :one
SELECT
  EXISTS (SELECT 1 FROM resumes r WHERE r.user_id = $1) AS has_resume,
  EXISTS (SELECT 1 FROM profiles p WHERE p.user_id = $1 AND p.confirmed_at IS NOT NULL) AS profile_confirmed,
  EXISTS (SELECT 1 FROM preferences pr WHERE pr.user_id = $1) AS has_preferences;
