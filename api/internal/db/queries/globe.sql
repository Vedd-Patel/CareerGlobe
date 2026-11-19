-- Globe: company pins inside the map bounds. Filters are optional (NULL = off).
-- A company passes when it has at least one open job that passes the job filters
-- (or, with no job filters and hiring_only off, always).

-- name: PinsInView :many
WITH job_cities AS (
  -- One row per job and city it is in (a job with no city counts once, with a NULL city).
  SELECT j.id AS job_id, j.company_id, jc.city
  FROM jobs j
  CROSS JOIN LATERAL unnest(CASE WHEN cardinality(j.cities) = 0 THEN ARRAY[NULL]::text[] ELSE j.cities END) AS jc(city)
  WHERE j.status = 'open'
    AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
    AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
    AND (sqlc.narg(posted_since)::timestamptz IS NULL OR j.first_seen >= sqlc.narg(posted_since))
), placed AS (
  -- Each job counts at the company location in its own city; one with no city, or
  -- in a city that has no pin, counts at the company's headquarters.
  SELECT jc.job_id, COALESCE(here.id, hq.id) AS location_id
  FROM job_cities jc
  LEFT JOIN LATERAL (SELECT l.id FROM company_locations l
                     WHERE l.company_id = jc.company_id AND l.geo IS NOT NULL AND lower(l.city) = lower(jc.city)
                     ORDER BY l.id LIMIT 1) here ON true
  LEFT JOIN LATERAL (SELECT l.id FROM company_locations l
                     WHERE l.company_id = jc.company_id AND l.geo IS NOT NULL AND l.is_hq
                     ORDER BY l.id LIMIT 1) hq ON true
), filtered AS (
  SELECT p.location_id,
         count(*)::int AS open_jobs,
         count(*) FILTER (WHERE m.score >= 60)::int AS matches
  FROM placed p
  LEFT JOIN matches m ON m.job_id = p.job_id AND m.user_id = sqlc.arg(user_id)::bigint
  WHERE p.location_id IS NOT NULL
  GROUP BY p.location_id
)
SELECT c.id AS company_id, c.name, c.slug, c.type, c.logo_url,
       l.id AS location_id, l.city, l.state, l.is_hq, l.address,
       ST_Y(l.geo::geometry)::float8 AS lat, ST_X(l.geo::geometry)::float8 AS lon,
       COALESCE(f.open_jobs, 0)::int AS open_jobs, COALESCE(f.matches, 0)::int AS matches
FROM company_locations l
JOIN companies c ON c.id = l.company_id
LEFT JOIN filtered f ON f.location_id = l.id
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
WITH job_cities AS (
  -- One row per job and city it is in (a job with no city counts once, with a NULL city).
  SELECT j.id AS job_id, j.company_id, jc.city
  FROM jobs j
  CROSS JOIN LATERAL unnest(CASE WHEN cardinality(j.cities) = 0 THEN ARRAY[NULL]::text[] ELSE j.cities END) AS jc(city)
  WHERE j.status = 'open'
    AND (sqlc.narg(category)::text IS NULL OR j.category = sqlc.narg(category) OR j.category_2 = sqlc.narg(category))
    AND (NOT sqlc.arg(remote_only)::bool OR j.work_mode = 'remote')
    AND (sqlc.narg(posted_since)::timestamptz IS NULL OR j.first_seen >= sqlc.narg(posted_since))
), placed AS (
  -- Each job counts at the company location in its own city; one with no city, or
  -- in a city that has no pin, counts at the company's headquarters.
  SELECT jc.job_id, COALESCE(here.id, hq.id) AS location_id
  FROM job_cities jc
  LEFT JOIN LATERAL (SELECT l.id FROM company_locations l
                     WHERE l.company_id = jc.company_id AND l.geo IS NOT NULL AND lower(l.city) = lower(jc.city)
                     ORDER BY l.id LIMIT 1) here ON true
  LEFT JOIN LATERAL (SELECT l.id FROM company_locations l
                     WHERE l.company_id = jc.company_id AND l.geo IS NOT NULL AND l.is_hq
                     ORDER BY l.id LIMIT 1) hq ON true
), filtered AS (
  SELECT p.location_id,
         count(*)::int AS open_jobs,
         count(*) FILTER (WHERE m.score >= 60)::int AS matches
  FROM placed p
  LEFT JOIN matches m ON m.job_id = p.job_id AND m.user_id = sqlc.arg(user_id)::bigint
  WHERE p.location_id IS NOT NULL
  GROUP BY p.location_id
), pins AS (
  SELECT c.id AS company_id, l.geo::geometry AS g,
         COALESCE(f.open_jobs, 0) AS open_jobs, COALESCE(f.matches, 0) AS matches
  FROM company_locations l
  JOIN companies c ON c.id = l.company_id
  LEFT JOIN filtered f ON f.location_id = l.id
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
-- The "Companies in view" list beside the globe: best matches first, or nearest first
-- to a point (the map centre or the member's position) when `nearest` is set.
SELECT t.id, t.name, t.slug, t.type, t.logo_url, t.city, t.address, t.open_jobs, t.matches FROM (
  SELECT c.id, c.name, c.slug, c.type, c.logo_url,
         (SELECT l2.city FROM company_locations l2 WHERE l2.company_id = c.id ORDER BY l2.is_hq DESC LIMIT 1) AS city,
         (SELECT l2.address FROM company_locations l2 WHERE l2.company_id = c.id AND l2.address IS NOT NULL
            ORDER BY l2.is_hq DESC LIMIT 1) AS address,
         (SELECT count(*) FROM jobs j WHERE j.company_id = c.id AND j.status = 'open')::int AS open_jobs,
         (SELECT count(*) FROM matches m JOIN jobs j ON j.id = m.job_id
          WHERE j.company_id = c.id AND j.status = 'open' AND m.user_id = sqlc.arg(user_id)::bigint AND m.score >= 60)::int AS matches,
         d.km AS km
  FROM companies c
  LEFT JOIN LATERAL (
    SELECT min(ST_Distance(l3.geo, ST_SetSRID(ST_MakePoint(sqlc.narg(near_lon)::float8, sqlc.narg(near_lat)::float8), 4326)::geography))
             / 1000.0 AS km
    FROM company_locations l3
    WHERE l3.company_id = c.id AND l3.geo IS NOT NULL
      AND sqlc.narg(near_lat)::float8 IS NOT NULL AND sqlc.narg(near_lon)::float8 IS NOT NULL
  ) d ON true
  WHERE EXISTS (
    SELECT 1 FROM company_locations l
    WHERE l.company_id = c.id AND l.geo IS NOT NULL
      AND l.geo::geometry && ST_MakeEnvelope(sqlc.arg(west)::float8, sqlc.arg(south)::float8,
                                             sqlc.arg(east)::float8, sqlc.arg(north)::float8, 4326))
    AND (sqlc.narg(company_type)::text IS NULL OR c.type = sqlc.narg(company_type) OR c.type = 'both')
) t
ORDER BY CASE WHEN sqlc.arg(nearest)::bool THEN t.km END ASC NULLS LAST, t.matches DESC, t.open_jobs DESC, t.name
LIMIT sqlc.arg(max_rows)::int OFFSET sqlc.arg(skip)::int;

-- name: CompanyDistances :many
-- Distance in km from a point to each company's nearest office.
SELECT l.company_id,
       (min(ST_Distance(l.geo, ST_SetSRID(ST_MakePoint(sqlc.arg(lon)::float8, sqlc.arg(lat)::float8), 4326)::geography)) / 1000.0)::float8 AS km
FROM company_locations l
WHERE l.company_id = ANY(sqlc.arg(company_ids)::bigint[]) AND l.geo IS NOT NULL
GROUP BY l.company_id;
