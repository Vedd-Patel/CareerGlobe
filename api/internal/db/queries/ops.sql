-- name: LastCrawlRun :one
SELECT * FROM crawl_runs ORDER BY started_at DESC LIMIT 1;

-- name: RequestCrawl :one
INSERT INTO crawl_triggers (reason) VALUES ($1) RETURNING *;

-- name: PendingCrawlTrigger :one
SELECT * FROM crawl_triggers WHERE started_at IS NULL ORDER BY requested_at LIMIT 1;
