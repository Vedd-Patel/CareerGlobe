-- name: LatestCrawlRuns :many
SELECT * FROM crawl_runs ORDER BY started_at DESC LIMIT $1;

-- name: FailingSources :many
SELECT s.id, s.company_id, c.name AS company, s.careers_url, s.ats_type, s.ats_slug, s.status, s.fail_count,
       s.last_error, s.last_attempt_at, s.last_ok_at, s.selectors
FROM careers_sources s JOIN companies c ON c.id = s.company_id
WHERE s.status IN ('broken', 'blocked', 'review') OR (s.last_error IS NOT NULL AND s.status <> 'disabled')
ORDER BY s.fail_count DESC, c.name;

-- name: UpdateSource :one
UPDATE careers_sources SET
  careers_url = sqlc.arg(careers_url)::text, ats_type = sqlc.narg(ats_type)::text,
  ats_slug = sqlc.narg(ats_slug)::text, selectors = sqlc.narg(selectors)::jsonb,
  status = sqlc.arg(status)::text, fail_count = CASE WHEN sqlc.arg(status)::text = 'active' THEN 0 ELSE fail_count END
WHERE id = sqlc.arg(id)::bigint
RETURNING *;

-- name: AdminCreateCompany :one
INSERT INTO companies (name, slug, website, type, origin) VALUES ($1, $2, $3, $4, 'admin') RETURNING *;

-- name: AdminAddLocation :exec
INSERT INTO company_locations (company_id, city, state, is_hq) VALUES ($1, $2, $3, true) ON CONFLICT DO NOTHING;

-- name: AdminAddSource :one
INSERT INTO careers_sources (company_id, careers_url, ats_type, ats_slug) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListCategoryRules :many
SELECT * FROM category_rules ORDER BY priority, keyword;

-- name: UpsertCategoryRule :one
INSERT INTO category_rules (keyword, category, priority) VALUES ($1, $2, $3)
ON CONFLICT (keyword) DO UPDATE SET category = EXCLUDED.category, priority = EXCLUDED.priority
RETURNING *;

-- name: DeleteCategoryRule :exec
DELETE FROM category_rules WHERE id = $1;

-- name: UnclassifiedTitles :many
SELECT title, count(*)::int AS jobs FROM jobs
WHERE status = 'open' AND (category IS NULL OR category = 'Other')
GROUP BY title ORDER BY jobs DESC, title LIMIT 200;

-- name: ListRemovalRequests :many
SELECT * FROM removal_requests ORDER BY (done_at IS NULL) DESC, received_at DESC LIMIT 200;

-- name: CreateRemovalRequest :one
INSERT INTO removal_requests (kind, email, user_id, company_id, note) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: MarkRemovalDone :exec
UPDATE removal_requests SET done_at = now(), note = $2 WHERE id = $1;

-- name: ListReports :many
SELECT r.*, c.name AS company_name, j.title AS job_title
FROM reports r LEFT JOIN companies c ON c.id = r.company_id LEFT JOIN jobs j ON j.id = r.job_id
ORDER BY (r.done_at IS NULL) DESC, r.created_at DESC LIMIT 200;

-- name: MarkReportDone :exec
UPDATE reports SET done_at = now() WHERE id = $1;
