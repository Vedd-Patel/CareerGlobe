-- name: JobsToClassify :many
-- Open jobs whose title or description changed since they were last classified.
SELECT id, title, COALESCE(description, '') AS description,
       md5(title || '|' || COALESCE(description, '')) AS text_hash
FROM jobs
WHERE status = 'open' AND classified_hash IS DISTINCT FROM md5(title || '|' || COALESCE(description, ''))
ORDER BY id
LIMIT $1;

-- name: SetJobClassification :exec
UPDATE jobs SET category = $2, category_2 = $3, seniority = $4, skills = $5, classified_hash = $6 WHERE id = $1;

-- name: UsersToMatch :many
-- Users with a confirmed profile (matching stays off until a resume is confirmed).
SELECT u.id, p.skills, p.titles, p.years_exp, p.current_city,
       COALESCE(pr.categories, '{}')::text[] AS categories, COALESCE(pr.cities, '{}')::text[] AS cities,
       COALESCE(pr.anywhere, false)::bool AS anywhere, COALESCE(pr.remote_ok, true)::bool AS remote_ok,
       COALESCE(pr.seniority, '{}')::text[] AS seniority, pr.min_salary
FROM users u
JOIN profiles p ON p.user_id = u.id AND p.confirmed_at IS NOT NULL
LEFT JOIN preferences pr ON pr.user_id = u.id
ORDER BY u.id;

-- name: CandidateJobs :many
-- Hard filters in SQL: open, in the user's categories (if any), in a preferred
-- city / remote / anywhere, and not below a stated minimum salary.
SELECT j.id, j.title, j.category, j.category_2, j.seniority, j.skills, j.cities, j.work_mode,
       j.salary_min, j.salary_max, j.first_seen
FROM jobs j
WHERE j.status = 'open'
  AND (cardinality(sqlc.arg(categories)::text[]) = 0 OR j.category = ANY(sqlc.arg(categories)::text[])
       OR j.category_2 = ANY(sqlc.arg(categories)::text[]))
  AND (sqlc.arg(anywhere)::bool
       OR j.cities && sqlc.arg(cities)::text[]
       OR (sqlc.arg(remote_ok)::bool AND j.work_mode = 'remote')
       OR cardinality(sqlc.arg(cities)::text[]) = 0)
  AND (sqlc.narg(min_salary)::int IS NULL OR j.salary_max IS NULL OR j.salary_max >= sqlc.narg(min_salary)::int);

-- name: UpsertMatch :exec
INSERT INTO matches (user_id, job_id, score, reasons, computed_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (user_id, job_id) DO UPDATE SET score = EXCLUDED.score, reasons = EXCLUDED.reasons, computed_at = now();

-- name: DeleteStaleMatches :execrows
-- Run in the same transaction as the user's upserts: rows not re-scored in it
-- have an older computed_at than now() (the transaction's start, the database
-- clock), and matches on closed jobs go too.
DELETE FROM matches m
WHERE m.user_id = $1 AND (m.computed_at < now() OR EXISTS (SELECT 1 FROM jobs j WHERE j.id = m.job_id AND j.status = 'closed'));

-- name: UsersDueForPush :many
-- Users whose push time (IST) has passed today, who have not had today's push,
-- and who have new matches scoring >= 60 not yet notified.
SELECT u.id,
       count(*)::int AS new_matches
FROM users u
JOIN preferences pr ON pr.user_id = u.id AND pr.push_enabled
JOIN matches m ON m.user_id = u.id AND m.score >= 60 AND m.notified_at IS NULL
JOIN jobs j ON j.id = m.job_id AND j.status = 'open'
WHERE (now() AT TIME ZONE 'Asia/Kolkata')::time >= pr.push_time
  AND (pr.last_push_at IS NULL OR (pr.last_push_at AT TIME ZONE 'Asia/Kolkata')::date < (now() AT TIME ZONE 'Asia/Kolkata')::date)
GROUP BY u.id;

-- name: PushTokensForUser :many
SELECT fcm_token, platform FROM push_tokens WHERE user_id = $1;

-- name: DeletePushTokenByValue :exec
DELETE FROM push_tokens WHERE fcm_token = $1;

-- name: MarkPushed :exec
UPDATE matches SET notified_at = now() WHERE user_id = $1 AND score >= 60 AND notified_at IS NULL;

-- name: SetLastPush :exec
UPDATE preferences SET last_push_at = now() WHERE user_id = $1;

-- name: AddNotification :exec
INSERT INTO notifications (user_id, kind, title, body, link) VALUES ($1, $2, $3, $4, $5);

-- name: SavedCompanyNewJobs :many
-- Jobs first seen since a time at companies a user saved, that match their categories.
SELECT sc.user_id, c.id AS company_id, c.name AS company_name, count(*)::int AS jobs
FROM saved_companies sc
JOIN companies c ON c.id = sc.company_id
JOIN jobs j ON j.company_id = c.id AND j.status = 'open' AND j.first_seen >= $1
LEFT JOIN preferences pr ON pr.user_id = sc.user_id
WHERE cardinality(COALESCE(pr.categories, '{}')) = 0 OR j.category = ANY(pr.categories) OR j.category_2 = ANY(pr.categories)
GROUP BY sc.user_id, c.id, c.name;

-- name: UpcomingNextSteps :many
-- Tracker reminders: next steps due in the next 24-48 hours (sent once, the day before).
SELECT a.id, a.user_id, a.next_step, a.next_step_at, j.title, c.name AS company_name
FROM applications a
JOIN jobs j ON j.id = a.job_id
JOIN companies c ON c.id = j.company_id
WHERE a.next_step_at >= now() + interval '12 hours' AND a.next_step_at < now() + interval '36 hours'
  AND NOT EXISTS (SELECT 1 FROM notifications n WHERE n.user_id = a.user_id AND n.kind = 'reminder'
                  AND n.link = '/tracker/' || a.id::text AND n.created_at > now() - interval '2 days');

-- name: CategoryRuleList :many
SELECT keyword, category, priority FROM category_rules;

-- name: SkillSynonymList :many
SELECT alias, canonical FROM skill_synonyms;

-- name: CityStates :many
SELECT DISTINCT city, state FROM company_locations WHERE city IS NOT NULL AND state IS NOT NULL;
