package nightly

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/push"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/testdb"
)

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func one[T any](t *testing.T, pool *pgxpool.Pool, sql string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

func setup(t *testing.T) (*pgxpool.Pool, int64) {
	pool := testdb.New(t, "careerglobe_nightly_test")
	exec(t, pool, `INSERT INTO access_codes (code_hash, label) VALUES ('\x01', 'test')`)
	exec(t, pool, `INSERT INTO users (firebase_uid, access_code_id) VALUES ('alice', 1)`)
	exec(t, pool, `INSERT INTO companies (name, slug) VALUES ('Acme', 'acme')`)
	exec(t, pool, `INSERT INTO company_locations (company_id, city, state) VALUES (1, 'Bengaluru', 'Karnataka'), (1, 'Mysuru', 'Karnataka')`)
	exec(t, pool, `INSERT INTO jobs (company_id, job_key, title, description, cities, work_mode, apply_url) VALUES
	  (1, 'a', 'Senior Backend Engineer', '<p>We use <b>Go</b>, Kafka, PostgreSQL and Kubernetes on AWS.</p>', '{Bengaluru}', 'hybrid', 'https://x/a'),
	  (1, 'b', 'Product Designer', '<p>Figma, prototyping and user research.</p>', '{Bengaluru}', NULL, 'https://x/b'),
	  (1, 'c', 'Backend Engineer', '<p>Go, Kafka, Redis, gRPC.</p>', '{Mysuru}', NULL, 'https://x/c'),
	  (1, 'd', 'Backend Engineer', '<p>Go, Kafka, Redis, gRPC.</p>', '{Delhi}', NULL, 'https://x/d'),
	  (1, 'e', 'Staff Backend Engineer (closed)', '<p>Go</p>', '{Bengaluru}', NULL, 'https://x/e')`)
	exec(t, pool, `UPDATE jobs SET status = 'closed' WHERE job_key = 'e'`)
	exec(t, pool, `INSERT INTO profiles (user_id, skills, titles, years_exp, current_city, confirmed_at)
	               VALUES (1, '{Go,Kafka,PostgreSQL,Kubernetes}', '{"Senior Backend Engineer"}', 7, 'Bengaluru', now())`)
	exec(t, pool, `INSERT INTO preferences (user_id, categories, cities, remote_ok, push_time)
	               VALUES (1, '{"Software Engineering"}', '{Bengaluru}', true, '00:00')`)
	return pool, 1
}

func TestClassifyMatchAlertsAndPush(t *testing.T) {
	pool, uid := setup(t)
	ctx := context.Background()

	n, err := Classify(ctx, pool)
	if err != nil || n != 4 {
		t.Fatalf("classified %d, %v (open jobs only)", n, err)
	}
	if again, _ := Classify(ctx, pool); again != 0 {
		t.Fatalf("unchanged jobs re-classified: %d", again)
	}
	if got := one[string](t, pool, `SELECT category FROM jobs WHERE job_key = 'b'`); got != "Design" {
		t.Fatalf("category = %q", got)
	}
	if got := one[[]string](t, pool, `SELECT skills FROM jobs WHERE job_key = 'a'`); len(got) < 5 {
		t.Fatalf("skills = %v", got)
	}

	users, kept, err := Match(ctx, pool, time.Now())
	if err != nil || users != 1 {
		t.Fatalf("match: %d users, %v", users, err)
	}
	// Hard filters (spec): other category (designer) and other cities (Mysuru, Delhi; not remote) are out.
	if kept != 1 {
		t.Fatalf("kept %d matches, want 1 (the Bengaluru job)", kept)
	}
	top := one[int](t, pool, `SELECT m.score FROM matches m JOIN jobs j ON j.id = m.job_id WHERE j.job_key = 'a'`)
	if top < 85 {
		t.Fatalf("exact-city senior role scored %d", top)
	}
	// "Anywhere in India": every city passes; exact city > same state > elsewhere.
	exec(t, pool, `UPDATE preferences SET anywhere = true`)
	if _, kept, _ := Match(ctx, pool, time.Now()); kept != 3 {
		t.Fatalf("anywhere: kept %d, want 3", kept)
	}
	score := func(key string) int {
		return one[int](t, pool, `SELECT m.score FROM matches m JOIN jobs j ON j.id = m.job_id WHERE j.job_key = $1`, key)
	}
	if !(score("a") > score("c") && score("c") > score("d")) {
		t.Fatalf("scores: Bengaluru %d, Mysuru %d, Delhi %d", score("a"), score("c"), score("d"))
	}
	exec(t, pool, `UPDATE preferences SET anywhere = false`)
	var reasons map[string]any
	_ = json.Unmarshal(one[[]byte](t, pool, `SELECT m.reasons FROM matches m JOIN jobs j ON j.id = m.job_id WHERE j.job_key = 'a'`), &reasons)
	if reasons["summary"] == "" || reasons["location"] != "Bengaluru · a city you chose" {
		t.Fatalf("reasons = %v", reasons)
	}

	// Narrower filters and closed jobs drop out on the next run.
	if _, kept, _ := Match(ctx, pool, time.Now()); kept != 1 {
		t.Fatalf("after narrowing: kept %d", kept)
	}
	if n := one[int](t, pool, `SELECT count(*) FROM matches`); n != 1 {
		t.Fatalf("stale matches left: %d", n)
	}

	// Saved company + tracker reminder, once a night.
	exec(t, pool, `INSERT INTO saved_companies (user_id, company_id) VALUES ($1, 1)`, uid)
	exec(t, pool, `INSERT INTO applications (user_id, job_id, status, next_step, next_step_at)
	               VALUES ($1, 1, 'interview', 'Tech round', now() + interval '24 hours')`, uid)
	al, err := Alerts(ctx, pool, time.Now().Add(-time.Hour))
	if err != nil || al.SavedAlerts != 1 || al.Reminders != 1 {
		t.Fatalf("alerts = %+v, %v", al, err)
	}
	if al, _ := Alerts(ctx, pool, time.Now().Add(-time.Hour)); al.Reminders != 0 {
		t.Fatal("the same reminder was added twice")
	}

	// Push: once a day, only with a new match >= 60; dead tokens are removed.
	exec(t, pool, `INSERT INTO push_tokens (user_id, platform, fcm_token) VALUES ($1, 'android', 'good'), ($1, 'web', 'dead')`, uid)
	rec := &push.Recorder{Dead: map[string]bool{"dead": true}}
	res, err := Push(ctx, pool, rec)
	if err != nil || res.Users != 1 || res.Sent != 1 || res.DeadTokens != 1 {
		t.Fatalf("push = %+v, %v", res, err)
	}
	if rec.Sent[0].Title != "1 new match this morning" || rec.Sent[0].Link != "/matches" {
		t.Fatalf("message = %+v", rec.Sent[0])
	}
	if n := one[int](t, pool, `SELECT count(*) FROM push_tokens`); n != 1 {
		t.Fatalf("dead token kept: %d tokens", n)
	}
	if res, _ := Push(ctx, pool, rec); res.Users != 0 {
		t.Fatal("pushed twice in one day")
	}
	if n := one[int](t, pool, `SELECT count(*) FROM notifications WHERE kind = 'matches'`); n != 1 {
		t.Fatalf("inbox items: %d", n)
	}
}

func TestNoConfirmedProfileNoMatching(t *testing.T) {
	pool, _ := setup(t)
	exec(t, pool, `UPDATE profiles SET confirmed_at = NULL`)
	if users, kept, err := Match(context.Background(), pool, time.Now()); err != nil || users != 0 || kept != 0 {
		t.Fatalf("users %d kept %d err %v", users, kept, err)
	}
}
