-- name: GetCodeByHash :one
SELECT * FROM access_codes WHERE code_hash = $1;

-- name: LockCode :one
SELECT * FROM access_codes WHERE id = $1 FOR UPDATE;

-- name: UseCode :exec
UPDATE access_codes SET used_count = used_count + 1 WHERE id = $1;

-- name: CreateCode :one
INSERT INTO access_codes (code_hash, label, max_uses, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListCodes :many
SELECT c.*, (SELECT count(*) FROM users u WHERE u.access_code_id = c.id)::int AS user_count
FROM access_codes c
ORDER BY c.created_at DESC;

-- name: SetCodeRevoked :one
UPDATE access_codes SET revoked = $2 WHERE id = $1 RETURNING *;

-- name: RecordAttempt :exec
INSERT INTO access_attempts (ip, device_id, success) VALUES ($1, $2, $3);

-- name: CountFailedAttempts :one
-- Failed code guesses from this IP or device since a time (rate limit).
SELECT count(*)::int FROM access_attempts
WHERE NOT success AND at >= sqlc.arg(since)::timestamptz
  AND (ip = sqlc.narg(ip)::inet OR device_id = sqlc.narg(device_id)::text);

-- name: CountAllFailedAttempts :one
SELECT count(*)::int FROM access_attempts WHERE NOT success AND at >= $1;

-- name: UseTicket :execrows
-- 0 rows: this ticket was already used.
INSERT INTO access_tickets_used (ticket_id, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: DeleteExpiredTickets :exec
DELETE FROM access_tickets_used WHERE expires_at < now();

-- name: PruneAttempts :exec
DELETE FROM access_attempts WHERE at < $1;
