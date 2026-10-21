-- name: ListMatches :many
-- Ranked matches (score >= min_score) on open jobs, with optional filters.
SELECT j.id AS job_id, j.title, j.category, j.cities, j.work_mode, j.posted_at, j.first_seen, j.status,
       c.id AS company_id, c.name AS company_name, c.logo_url,
       m.score, m.reasons, m.seen, m.computed_at,
       EXISTS (SELECT 1 FROM applications a WHERE a.user_id = m.user_id AND a.job_id = j.id) AS saved
FROM matches m
JOIN jobs j ON j.id = m.job_id
JOIN companies c ON c.id = j.company_id
WHERE m.user_id = sqlc.arg(user_id)::bigint AND j.status = 'open' AND m.score >= sqlc.arg(min_score)::int
  AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
  AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
  AND (sqlc.narg(company_type)::text IS NULL OR c.type = sqlc.narg(company_type) OR c.type = 'both')
  AND (sqlc.narg(city)::text IS NULL OR sqlc.narg(city) = ANY(j.cities))
ORDER BY
  CASE WHEN sqlc.arg(sort_newest)::bool THEN extract(epoch FROM j.first_seen) ELSE m.score END DESC,
  m.score DESC, j.first_seen DESC, j.id
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(skip)::int;

-- name: MarkMatchSeen :exec
UPDATE matches SET seen = true WHERE user_id = $1 AND job_id = $2;

-- name: CountNewMatches :one
SELECT count(*)::int FROM matches m JOIN jobs j ON j.id = m.job_id
WHERE m.user_id = $1 AND j.status = 'open' AND m.score >= 60 AND NOT m.seen;
