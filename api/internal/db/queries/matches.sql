-- name: ListMatches :many
-- Ranked matches (score >= min_score) on open jobs, with optional filters.
SELECT j.id AS job_id, j.title, j.category, j.cities, j.work_mode, j.posted_at, j.first_seen, j.status,
       j.employment_type, j.years_min, j.years_max, j.salary_min, j.salary_max,
       c.id AS company_id, c.name AS company_name, c.logo_url,
       m.score, m.reasons, m.seen, m.computed_at,
       EXISTS (SELECT 1 FROM applications a WHERE a.user_id = m.user_id AND a.job_id = j.id) AS saved
FROM matches m
JOIN jobs j ON j.id = m.job_id
JOIN companies c ON c.id = j.company_id
WHERE m.user_id = sqlc.arg(user_id)::bigint AND j.status = 'open' AND m.score >= sqlc.arg(min_score)::int
  AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
  AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
  AND (sqlc.narg(employment)::text[] IS NULL OR j.employment_type = ANY(sqlc.narg(employment)::text[]))
  AND (sqlc.narg(exp_min)::int IS NULL OR (COALESCE(j.years_min, 0) <= sqlc.narg(exp_max)::int AND COALESCE(j.years_max, 99) >= sqlc.narg(exp_min)::int))
  AND (sqlc.narg(min_salary)::int IS NULL OR j.salary_max >= sqlc.narg(min_salary)::int)
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
