-- Globe: company pins inside the map bounds. Filters are optional (NULL = off).
-- A company passes when it has at least one open job that passes the job filters
-- (or, with no job filters and hiring_only off, always).

-- name: PinsInView :many
WITH filtered AS (
  SELECT j.company_id,
         count(*)::int AS open_jobs,
         count(*) FILTER (WHERE m.score >= 60)::int AS matches
  FROM jobs j
  LEFT JOIN matches m ON m.job_id = j.id AND m.user_id = sqlc.arg(user_id)::bigint
  WHERE j.status = 'open'
    AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
    AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
    AND (sqlc.narg(posted_since)::timestamptz IS NULL OR j.first_seen >= sqlc.narg(posted_since))
  GROUP BY j.company_id
)
SELECT c.id AS company_id, c.name, c.slug, c.type, c.logo_url,
       l.id AS location_id, l.city, l.state, l.is_hq,
       ST_Y(l.geo::geometry)::float8 AS lat, ST_X(l.geo::geometry)::float8 AS lon,
       COALESCE(f.open_jobs, 0)::int AS open_jobs, COALESCE(f.matches, 0)::int AS matches
FROM company_locations l
JOIN companies c ON c.id = l.company_id
LEFT JOIN filtered f ON f.company_id = c.id
WHERE l.geo IS NOT NULL
  AND l.geo::geometry && ST_MakeEnvelope(sqlc.arg(west)::float8, sqlc.arg(south)::float8,
                                         sqlc.arg(east)::float8, sqlc.arg(north)::float8, 4326)
  AND (sqlc.narg(company_type)::text IS NULL OR c.type = sqlc.narg(company_type) OR c.type = 'both')
  AND (NOT sqlc.arg(hiring_only)::bool OR f.open_jobs > 0)
  AND (NOT sqlc.arg(matches_only)::bool OR f.matches > 0)
  AND (NOT sqlc.arg(job_filters)::bool OR f.open_jobs > 0)
ORDER BY COALESCE(f.matches, 0) DESC, COALESCE(f.open_jobs, 0) DESC, c.name
LIMIT sqlc.arg(max_pins)::int;

-- name: ClustersInView :many
-- Zoomed out: pins snapped to a grid of `cell` degrees, one row per occupied cell.
WITH filtered AS (
  SELECT j.company_id,
         count(*)::int AS open_jobs,
         count(*) FILTER (WHERE m.score >= 60)::int AS matches
  FROM jobs j
  LEFT JOIN matches m ON m.job_id = j.id AND m.user_id = sqlc.arg(user_id)::bigint
  WHERE j.status = 'open'
    AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
    AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
    AND (sqlc.narg(posted_since)::timestamptz IS NULL OR j.first_seen >= sqlc.narg(posted_since))
  GROUP BY j.company_id
), pins AS (
  SELECT c.id AS company_id, l.geo::geometry AS g,
         COALESCE(f.open_jobs, 0) AS open_jobs, COALESCE(f.matches, 0) AS matches
  FROM company_locations l
  JOIN companies c ON c.id = l.company_id
  LEFT JOIN filtered f ON f.company_id = c.id
  WHERE l.geo IS NOT NULL
    AND l.geo::geometry && ST_MakeEnvelope(sqlc.arg(west)::float8, sqlc.arg(south)::float8,
                                           sqlc.arg(east)::float8, sqlc.arg(north)::float8, 4326)
    AND (sqlc.narg(company_type)::text IS NULL OR c.type = sqlc.narg(company_type) OR c.type = 'both')
    AND (NOT sqlc.arg(hiring_only)::bool OR f.open_jobs > 0)
    AND (NOT sqlc.arg(matches_only)::bool OR f.matches > 0)
    AND (NOT sqlc.arg(job_filters)::bool OR f.open_jobs > 0)
)
SELECT ST_Y(ST_Centroid(ST_Collect(g)))::float8 AS lat,
       ST_X(ST_Centroid(ST_Collect(g)))::float8 AS lon,
       count(DISTINCT company_id)::int AS companies,
       sum(open_jobs)::int AS open_jobs,
       sum(matches)::int AS matches,
       (array_agg(company_id ORDER BY matches DESC, open_jobs DESC))[1]::bigint AS top_company_id
FROM pins
GROUP BY ST_SnapToGrid(g, sqlc.arg(cell)::float8)
ORDER BY companies DESC
LIMIT sqlc.arg(max_pins)::int;

-- name: CompaniesInView :many
-- The "Companies in view" list beside the globe, sorted by matches.
SELECT c.id, c.name, c.slug, c.type, c.logo_url,
       (SELECT l2.city FROM company_locations l2 WHERE l2.company_id = c.id ORDER BY l2.is_hq DESC LIMIT 1) AS city,
       (SELECT count(*) FROM jobs j WHERE j.company_id = c.id AND j.status = 'open')::int AS open_jobs,
       (SELECT count(*) FROM matches m JOIN jobs j ON j.id = m.job_id
        WHERE j.company_id = c.id AND j.status = 'open' AND m.user_id = sqlc.arg(user_id)::bigint AND m.score >= 60)::int AS matches
FROM companies c
WHERE EXISTS (
  SELECT 1 FROM company_locations l
  WHERE l.company_id = c.id AND l.geo IS NOT NULL
    AND l.geo::geometry && ST_MakeEnvelope(sqlc.arg(west)::float8, sqlc.arg(south)::float8,
                                           sqlc.arg(east)::float8, sqlc.arg(north)::float8, 4326))
  AND (sqlc.narg(company_type)::text IS NULL OR c.type = sqlc.narg(company_type) OR c.type = 'both')
ORDER BY matches DESC, open_jobs DESC, c.name
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(skip)::int;
