-- name: GetJob :one
SELECT j.id, j.company_id, j.title, j.category, j.category_2, j.seniority, j.description, j.cities, j.work_mode,
       j.salary_min, j.salary_max, j.apply_url, j.posted_at, j.first_seen, j.last_seen, j.closed_at, j.status,
       c.name AS company_name, c.slug AS company_slug, c.logo_url AS company_logo, c.type AS company_type
FROM jobs j JOIN companies c ON c.id = j.company_id
WHERE j.id = $1;

-- name: GetMatch :one
SELECT score, reasons, computed_at, seen FROM matches WHERE user_id = $1 AND job_id = $2;

-- name: SimilarOpenJobs :many
-- For a closed job: open jobs in the same category, same company first.
SELECT j.id, j.title, j.cities, j.work_mode, j.first_seen, c.id AS company_id, c.name AS company_name, c.logo_url,
       m.score
FROM jobs j JOIN companies c ON c.id = j.company_id
LEFT JOIN matches m ON m.job_id = j.id AND m.user_id = sqlc.arg(user_id)::bigint
WHERE j.status = 'open' AND j.id <> sqlc.arg(job_id)::bigint
  AND (j.category = sqlc.narg(category)::text OR j.company_id = sqlc.arg(company_id)::bigint)
ORDER BY (j.company_id = sqlc.arg(company_id)::bigint) DESC, m.score DESC NULLS LAST, j.first_seen DESC
LIMIT 10;

-- name: SearchCompanies :many
SELECT id, name, slug, type, logo_url FROM companies
WHERE name ILIKE '%' || sqlc.arg(q)::text || '%'
ORDER BY (lower(name) = lower(sqlc.arg(q)::text)) DESC, (name ILIKE sqlc.arg(q)::text || '%') DESC, name
LIMIT 10;

-- name: SearchJobs :many
SELECT j.id, j.title, j.cities, j.work_mode, c.id AS company_id, c.name AS company_name, c.logo_url
FROM jobs j JOIN companies c ON c.id = j.company_id
WHERE j.status = 'open' AND j.title ILIKE '%' || sqlc.arg(q)::text || '%'
ORDER BY j.first_seen DESC
LIMIT 20;

-- name: SearchCities :many
SELECT l.city, count(DISTINCT l.company_id)::int AS companies,
       avg(ST_Y(l.geo::geometry))::float8 AS lat, avg(ST_X(l.geo::geometry))::float8 AS lon
FROM company_locations l
WHERE l.city ILIKE sqlc.arg(q)::text || '%' AND l.geo IS NOT NULL
GROUP BY l.city ORDER BY companies DESC LIMIT 5;

-- name: PublicStats :one
SELECT (SELECT count(*) FROM companies c WHERE EXISTS (
          SELECT 1 FROM careers_sources s WHERE s.company_id = c.id AND s.status <> 'disabled'))::int AS companies,
       (SELECT count(*) FROM jobs WHERE status = 'open')::int AS open_jobs,
       (SELECT r.finished_at FROM crawl_runs r WHERE r.finished_at IS NOT NULL ORDER BY r.finished_at DESC LIMIT 1) AS last_crawl_at;

-- name: CityList :many
SELECT l.city, l.state, count(DISTINCT l.company_id)::int AS companies
FROM company_locations l WHERE l.city IS NOT NULL
GROUP BY l.city, l.state ORDER BY companies DESC, l.city LIMIT 200;
