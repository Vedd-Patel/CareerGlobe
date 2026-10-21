-- name: GetCompany :one
SELECT c.*,
       (SELECT s.careers_url FROM careers_sources s WHERE s.company_id = c.id ORDER BY s.id LIMIT 1) AS careers_url,
       (SELECT s.last_ok_at FROM careers_sources s WHERE s.company_id = c.id AND s.last_ok_at IS NOT NULL
        ORDER BY s.last_ok_at DESC LIMIT 1) AS last_checked_at,
       EXISTS (SELECT 1 FROM saved_companies sc WHERE sc.company_id = c.id AND sc.user_id = sqlc.arg(user_id)::bigint) AS saved
FROM companies c WHERE c.id = sqlc.arg(id)::bigint;

-- name: CompanyLocations :many
SELECT id, city, state, country, is_hq, geo_approximate,
       ST_Y(geo::geometry)::float8 AS lat, ST_X(geo::geometry)::float8 AS lon
FROM company_locations WHERE company_id = $1 AND geo IS NOT NULL
ORDER BY is_hq DESC, city;

-- name: CompanyOpenJobs :many
SELECT j.id, j.title, j.category, j.category_2, j.seniority, j.cities, j.work_mode, j.posted_at, j.first_seen,
       m.score
FROM jobs j
LEFT JOIN matches m ON m.job_id = j.id AND m.user_id = sqlc.arg(user_id)::bigint
WHERE j.company_id = sqlc.arg(company_id)::bigint AND j.status = 'open'
ORDER BY j.category NULLS LAST, m.score DESC NULLS LAST, j.first_seen DESC
LIMIT 500;

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
