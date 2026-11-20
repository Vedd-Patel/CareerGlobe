-- name: GetCompany :one
SELECT c.*,
       (SELECT s.careers_url FROM careers_sources s WHERE s.company_id = c.id ORDER BY s.id LIMIT 1) AS careers_url,
       (SELECT s.last_ok_at FROM careers_sources s WHERE s.company_id = c.id AND s.last_ok_at IS NOT NULL
        ORDER BY s.last_ok_at DESC LIMIT 1) AS last_checked_at,
       EXISTS (SELECT 1 FROM saved_companies sc WHERE sc.company_id = c.id AND sc.user_id = sqlc.arg(user_id)::bigint) AS saved
FROM companies c WHERE c.id = sqlc.arg(id)::bigint;

-- name: CompanyLocations :many
SELECT id, city, state, country, is_hq, geo_approximate, address,
       ST_Y(geo::geometry)::float8 AS lat, ST_X(geo::geometry)::float8 AS lon
FROM company_locations WHERE company_id = $1 AND geo IS NOT NULL
ORDER BY is_hq DESC, city;

-- name: CompanyOpenJobs :many
-- A company's open jobs that pass the job filters (and the city, when one is given).
SELECT j.id, j.title, j.category, j.category_2, j.seniority, j.cities, j.work_mode, j.posted_at, j.first_seen,
       j.employment_type, j.years_min, j.years_max, j.salary_min, j.salary_max,
       m.score
FROM jobs j
LEFT JOIN matches m ON m.job_id = j.id AND m.user_id = sqlc.arg(user_id)::bigint
WHERE j.company_id = sqlc.arg(company_id)::bigint AND j.status = 'open'
  AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
  AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
  AND (sqlc.narg(posted_since)::timestamptz IS NULL OR j.first_seen >= sqlc.narg(posted_since))
  AND (sqlc.narg(employment)::text[] IS NULL OR j.employment_type = ANY(sqlc.narg(employment)::text[]))
  AND (sqlc.narg(exp_min)::int IS NULL OR (COALESCE(j.years_min, 0) <= sqlc.narg(exp_max)::int AND COALESCE(j.years_max, 99) >= sqlc.narg(exp_min)::int))
  AND (sqlc.narg(min_salary)::int IS NULL OR j.salary_max >= sqlc.narg(min_salary)::int)
  AND (sqlc.narg(city)::text IS NULL OR EXISTS (SELECT 1 FROM unnest(j.cities) c WHERE lower(c) = lower(sqlc.narg(city))))
ORDER BY j.category NULLS LAST, m.score DESC NULLS LAST, j.first_seen DESC
LIMIT 500;

-- name: CountCompanyOpenJobs :one
SELECT count(*)::int FROM jobs WHERE company_id = $1 AND status = 'open';

-- name: SaveCompany :exec
INSERT INTO saved_companies (user_id, company_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UnsaveCompany :exec
DELETE FROM saved_companies WHERE user_id = $1 AND company_id = $2;

-- name: SavedCompanies :many
SELECT c.id, c.name, c.slug, c.type, c.logo_url, sc.created_at,
       (SELECT count(*) FROM jobs j WHERE j.company_id = c.id AND j.status = 'open')::int AS open_jobs
FROM saved_companies sc JOIN companies c ON c.id = sc.company_id
WHERE sc.user_id = $1 ORDER BY sc.created_at DESC;

-- name: CreateReport :exec
INSERT INTO reports (user_id, company_id, job_id, message) VALUES ($1, $2, $3, $4);
