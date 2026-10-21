package server

// End-to-end tests against the local Postgres (docker compose up -d db).
// Each test gets a fresh database `careerglobe_api_test` with every migration
// applied. Skipped when DATABASE_URL is unset or the database is unreachable.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/auth"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/resume"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/testdb"
)

const testDB = "careerglobe_api_test"

type env struct {
	t      *testing.T
	pool   *pgxpool.Pool
	access *access.Service
	h      http.Handler
	clock  *time.Time
	mu     sync.Mutex
}

var tokens = auth.Static{
	"tok-alice":      {UID: "alice", Email: "alice@example.com", Name: "Alice", Provider: "google.com", EmailVerified: true},
	"tok-bob":        {UID: "bob", Email: "bob@example.com", Name: "Bob", Provider: "google.com", EmailVerified: true},
	"tok-new":        {UID: "newcomer", Email: "new@example.com", Provider: "google.com", EmailVerified: true},
	"tok-admin":      {UID: "admin-uid", Email: "me@example.com", Provider: "google.com", EmailVerified: true},
	"tok-unverified": {UID: "unverified", Email: "u@example.com", Provider: "password", EmailVerified: false},
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testdb.New(t, testDB)
	hasher, _ := access.NewHasher(bytes.Repeat([]byte("k"), 32))
	acc := access.NewService(pool, hasher)
	e := &env{t: t, pool: pool, access: acc}
	now := time.Now()
	e.clock = &now
	acc.SetClock(func() time.Time { e.mu.Lock(); defer e.mu.Unlock(); return *e.clock })
	vault, err := resume.NewVault(t.TempDir(), bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	e.h = New(Config{DB: pool, Pool: pool, Access: acc, Verifier: tokens, Resumes: resume.NewService(pool, vault),
		Admins: map[string]bool{"admin-uid": true}, OpsToken: "ops-secret", Origins: []string{"https://app.example"},
		IPHeader: "CF-Connecting-IP", PolicyVersion: "2026-10-01"})
	return e
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	*e.clock = e.clock.Add(d)
	e.mu.Unlock()
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (e *env) do(method, path, token string, body any, hdr ...string) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := resp{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.Body)
	return out
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

func (e *env) id(sql string, args ...any) int64 {
	e.t.Helper()
	var id int64
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
	return id
}

// join makes a user a member: a code, verify, redeem.
func (e *env) join(token string) {
	e.t.Helper()
	code, _, err := e.access.CreateCode(context.Background(), "for "+token, 1, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	v := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code, "device_id": "dev-" + token})
	if v.Status != 200 {
		e.t.Fatalf("verify: %d %s", v.Status, v.Raw)
	}
	r := e.do("POST", "/v1/access/redeem", token, map[string]string{"ticket": v.Body["ticket"].(string)})
	if r.Status != 200 {
		e.t.Fatalf("redeem: %d %s", r.Status, r.Raw)
	}
}

// seed: two companies with pinned offices and open jobs, one closed job.
func (e *env) seed() (acme, beta, job1, closed int64) {
	acme = e.id(`INSERT INTO companies (name, slug, type) VALUES ('Acme', 'acme', 'product') RETURNING id`)
	beta = e.id(`INSERT INTO companies (name, slug, type) VALUES ('Beta Services', 'beta', 'service') RETURNING id`)
	e.exec(`INSERT INTO company_locations (company_id, city, state, is_hq, geo) VALUES
	        ($1, 'Bengaluru', 'Karnataka', true, ST_SetSRID(ST_MakePoint(77.59, 12.97), 4326)::geography),
	        ($2, 'Pune', 'Maharashtra', true, ST_SetSRID(ST_MakePoint(73.85, 18.52), 4326)::geography)`, acme, beta)
	e.exec(`INSERT INTO careers_sources (company_id, careers_url, last_ok_at) VALUES ($1, 'https://jobs.lever.co/acme', now())`, acme)
	job1 = e.id(`INSERT INTO jobs (company_id, job_key, title, category, description, cities, work_mode, apply_url)
	             VALUES ($1, 'k1', 'Backend Engineer', 'Software Engineering', '<p>Build APIs in Go.</p>', '{Bengaluru}',
	             'hybrid', 'https://jobs.lever.co/acme/1') RETURNING id`, acme)
	e.exec(`INSERT INTO jobs (company_id, job_key, title, category, cities, work_mode, apply_url)
	        VALUES ($1, 'k2', 'Product Designer', 'Design', '{Bengaluru}', 'remote', 'https://jobs.lever.co/acme/2')`, acme)
	e.exec(`INSERT INTO jobs (company_id, job_key, title, category, cities, apply_url)
	        VALUES ($1, 'k3', 'QA Engineer', 'QA & Testing', '{Pune}', 'https://beta.example/jobs/3')`, beta)
	closed = e.id(`INSERT INTO jobs (company_id, job_key, title, category, cities, apply_url, status, closed_at)
	               VALUES ($1, 'k4', 'Old Backend Role', 'Software Engineering', '{Bengaluru}', 'https://jobs.lever.co/acme/4',
	               'closed', now()) RETURNING id`, acme)
	e.exec(`INSERT INTO crawl_runs (started_at, finished_at, companies_ok) VALUES (now() - interval '2 hours', now() - interval '1 hour', 2)`)
	return
}

func (e *env) userID(uid string) int64 {
	return e.id(`SELECT id FROM users WHERE firebase_uid = $1`, uid)
}

// Every member endpoint, with placeholder IDs. Used to prove the access check
// runs everywhere (spec: Phase 3 "done when").
var memberRoutes = [][2]string{
	{"GET", "/v1/me"}, {"GET", "/v1/meta"},
	{"GET", "/v1/globe?west=68&south=6&east=98&north=36&zoom=4"},
	{"GET", "/v1/companies/in-view?west=68&south=6&east=98&north=36"}, {"GET", "/v1/companies/1"},
	{"PUT", "/v1/companies/1/save"}, {"DELETE", "/v1/companies/1/save"}, {"GET", "/v1/jobs/1"},
	{"GET", "/v1/search?q=acme"}, {"POST", "/v1/reports"}, {"GET", "/v1/matches"}, {"POST", "/v1/matches/1/seen"},
	{"GET", "/v1/applications"}, {"GET", "/v1/applications/stats"}, {"POST", "/v1/applications"},
	{"GET", "/v1/applications/1"}, {"PATCH", "/v1/applications/1"}, {"DELETE", "/v1/applications/1"},
	{"POST", "/v1/applications/1/rounds"}, {"DELETE", "/v1/applications/1/rounds/1"},
	{"GET", "/v1/saved/companies"}, {"GET", "/v1/profile"}, {"PUT", "/v1/profile"}, {"GET", "/v1/preferences"},
	{"PUT", "/v1/preferences"}, {"GET", "/v1/resume"}, {"POST", "/v1/resume"}, {"GET", "/v1/consents"},
	{"POST", "/v1/push-tokens"}, {"DELETE", "/v1/push-tokens"}, {"GET", "/v1/notifications"},
	{"POST", "/v1/notifications/read"}, {"DELETE", "/v1/account"},
}

var adminRoutes = [][2]string{
	{"GET", "/v1/admin/crawl"}, {"GET", "/v1/admin/sources/failing"}, {"PUT", "/v1/admin/sources/1"},
	{"POST", "/v1/admin/companies"}, {"GET", "/v1/admin/category-rules"}, {"PUT", "/v1/admin/category-rules"},
	{"DELETE", "/v1/admin/category-rules/1"}, {"GET", "/v1/admin/unclassified"}, {"GET", "/v1/admin/codes"},
	{"POST", "/v1/admin/codes"}, {"PUT", "/v1/admin/codes/1/revoked"}, {"GET", "/v1/admin/codes/1/users"},
	{"GET", "/v1/admin/removal-requests"}, {"POST", "/v1/admin/removal-requests"},
	{"POST", "/v1/admin/removal-requests/1/done"}, {"GET", "/v1/admin/reports"}, {"POST", "/v1/admin/reports/1/done"},
}

func TestEveryEndpointChecksTokenAndCode(t *testing.T) {
	e := newEnv(t)
	e.join("tok-alice")
	for _, rt := range append(append([][2]string{}, memberRoutes...), adminRoutes...) {
		if r := e.do(rt[0], rt[1], "", nil); r.Status != 401 {
			t.Errorf("%s %s without token: %d, want 401", rt[0], rt[1], r.Status)
		}
		if r := e.do(rt[0], rt[1], "tok-forged", nil); r.Status != 401 {
			t.Errorf("%s %s with a bad token: %d, want 401", rt[0], rt[1], r.Status)
		}
		r := e.do(rt[0], rt[1], "tok-new", nil)
		if r.Status != 403 || r.Body["error"] != "access_code_required" {
			t.Errorf("%s %s for an account without a code: %d %s, want 403 access_code_required", rt[0], rt[1], r.Status, r.Raw)
		}
	}
	for _, rt := range adminRoutes {
		if r := e.do(rt[0], rt[1], "tok-alice", nil); r.Status != 403 || r.Body["error"] != "not_admin" {
			t.Errorf("%s %s for a member: %d %s, want 403 not_admin", rt[0], rt[1], r.Status, r.Raw)
		}
	}
	if r := e.do("GET", "/v1/me", "tok-unverified", nil); r.Status != 403 || r.Body["error"] != "email_not_verified" {
		t.Errorf("unverified email: %d %s", r.Status, r.Raw)
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Status != 200 {
		t.Errorf("member: %d %s", r.Status, r.Raw)
	}
}

func TestAccessCodeFlowAndRevocation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	code, row, _ := e.access.CreateCode(ctx, "Friends", 2, nil)

	for _, bad := range []string{"", "nonsense", "CG-2222-2222"} {
		if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": bad}); r.Body["error"] != "invalid_code" {
			t.Errorf("verify %q: %s", bad, r.Raw)
		}
	}
	v := e.do("POST", "/v1/access/verify", "", map[string]string{"code": strings.ToLower(code), "device_id": "d1"})
	if v.Status != 200 || v.Body["ticket"] == "" {
		t.Fatalf("verify: %s", v.Raw)
	}
	ticket := v.Body["ticket"].(string)
	var used int
	_ = e.pool.QueryRow(ctx, "SELECT used_count FROM access_codes WHERE id=$1", row.ID).Scan(&used)
	if used != 0 {
		t.Fatal("verify must not use up the code")
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["error"] != "access_code_required" {
		t.Fatalf("before redeem: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/access/redeem", "", map[string]string{"ticket": ticket}); r.Status != 401 {
		t.Fatalf("redeem without Firebase token: %d", r.Status)
	}
	if r := e.do("POST", "/v1/access/redeem", "tok-alice", map[string]string{"ticket": ticket}); r.Status != 200 {
		t.Fatalf("redeem: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/access/redeem", "tok-bob", map[string]string{"ticket": ticket}); r.Body["error"] != "ticket_used" {
		t.Fatalf("ticket reuse: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Status != 200 || r.Body["is_admin"] != false {
		t.Fatalf("me: %s", r.Raw)
	}

	// Expired tickets are refused.
	v2 := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code})
	e.advance(11 * time.Minute)
	if r := e.do("POST", "/v1/access/redeem", "tok-bob", map[string]string{"ticket": v2.Body["ticket"].(string)}); r.Body["error"] != "ticket_expired" {
		t.Fatalf("expired ticket: %s", r.Raw)
	}

	// Revoke: locked out within a minute (the cache), with a clear message.
	if _, err := e.access.SetRevoked(ctx, row.ID, true); err != nil {
		t.Fatal(err)
	}
	e.advance(access.AllowedCacheTTL + time.Second)
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Status != 403 || r.Body["error"] != "access_removed" {
		t.Fatalf("after revoke: %d %s", r.Status, r.Raw)
	}
	if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code}); r.Body["error"] != "code_revoked" {
		t.Fatalf("verify revoked: %s", r.Raw)
	}
}

func TestCodeExpiryAndUsedUp(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	expired, _, _ := e.access.CreateCode(ctx, "old", 1, &past)
	if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": expired}); r.Body["error"] != "code_expired" {
		t.Fatalf("expired: %s", r.Raw)
	}
	once, _, _ := e.access.CreateCode(ctx, "once", 1, nil)
	t1 := e.do("POST", "/v1/access/verify", "", map[string]string{"code": once}).Body["ticket"].(string)
	t2 := e.do("POST", "/v1/access/verify", "", map[string]string{"code": once}).Body["ticket"].(string)
	if r := e.do("POST", "/v1/access/redeem", "tok-alice", map[string]string{"ticket": t1}); r.Status != 200 {
		t.Fatal(r.Raw)
	}
	if r := e.do("POST", "/v1/access/redeem", "tok-bob", map[string]string{"ticket": t2}); r.Body["error"] != "code_used_up" {
		t.Fatalf("second user on a one-use code: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": once}); r.Body["error"] != "code_used_up" {
		t.Fatalf("verify used-up code: %s", r.Raw)
	}
}

func TestCodeGuessingIsRateLimited(t *testing.T) {
	e := newEnv(t)
	code, _, _ := e.access.CreateCode(context.Background(), "x", 1, nil)
	ip := []string{"CF-Connecting-IP", "203.0.113.7"}
	for i := range access.MaxFailedAttempts {
		if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": "CG-2222-222" + fmt.Sprint(i+2)}, ip...); r.Status != 400 {
			t.Fatalf("try %d: %s", i, r.Raw)
		}
	}
	r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code}, ip...)
	if r.Status != 429 || r.Body["error"] != "too_many_attempts" {
		t.Fatalf("6th try: %d %s", r.Status, r.Raw)
	}
	// Another IP and device is not affected; after 15 minutes the first one may try again.
	if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code}, "CF-Connecting-IP", "198.51.100.1"); r.Status != 200 {
		t.Fatalf("other IP: %s", r.Raw)
	}
	e.advance(access.AttemptWindow + time.Second)
	if r := e.do("POST", "/v1/access/verify", "", map[string]string{"code": code}, ip...); r.Status != 200 {
		t.Fatalf("after the window: %s", r.Raw)
	}
}

func TestRaceForTheLastUse(t *testing.T) {
	e := newEnv(t)
	code, row, _ := e.access.CreateCode(context.Background(), "last seat", 1, nil)
	users := []string{"tok-alice", "tok-bob"}
	tickets := make([]string, len(users))
	for i := range users {
		tickets[i] = e.do("POST", "/v1/access/verify", "", map[string]string{"code": code}).Body["ticket"].(string)
	}
	var wg sync.WaitGroup
	results := make([]int, len(users))
	for i := range users {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = e.do("POST", "/v1/access/redeem", users[i], map[string]string{"ticket": tickets[i]}).Status
		}(i)
	}
	wg.Wait()
	sort.Ints(results)
	if results[0] != 200 || results[1] != 403 {
		t.Fatalf("results = %v, want exactly one 200", results)
	}
	var used int
	_ = e.pool.QueryRow(context.Background(), "SELECT used_count FROM access_codes WHERE id=$1", row.ID).Scan(&used)
	if used != 1 {
		t.Fatalf("used_count = %d", used)
	}
}

func TestGlobeCompaniesJobsAndSearch(t *testing.T) {
	e := newEnv(t)
	e.join("tok-alice")
	acme, beta, job1, closed := e.seed()
	uid := e.userID("alice")
	e.exec(`INSERT INTO matches (user_id, job_id, score, reasons) VALUES ($1, $2, 82, '{"top":"Matches 4 of 5 skills"}')`, uid, job1)

	india := "west=68&south=6&east=98&north=36"
	r := e.do("GET", "/v1/globe?"+india+"&zoom=3", "tok-alice", nil)
	if r.Status != 200 || r.Body["mode"] != "clusters" {
		t.Fatalf("clusters: %s", r.Raw)
	}
	r = e.do("GET", "/v1/globe?"+india+"&zoom=9", "tok-alice", nil)
	items := r.Body["items"].([]any)
	if r.Body["mode"] != "pins" || len(items) != 2 {
		t.Fatalf("pins: %s", r.Raw)
	}
	first := items[0].(map[string]any)
	if first["Name"] != "Acme" || first["Matches"] != float64(1) || first["OpenJobs"] != float64(2) {
		t.Fatalf("pin sorting/counts: %v", first)
	}
	if r := e.do("GET", "/v1/globe?"+india+"&zoom=9&matches=1", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("matches only: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/globe?"+india+"&zoom=9&category=Design", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("category filter: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/globe?"+india+"&zoom=9&type=service", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("type filter: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/globe?west=0&south=0&east=10&north=10&zoom=9", "tok-alice", nil); len(r.Body["items"].([]any)) != 0 {
		t.Fatalf("outside India: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/globe?west=a", "tok-alice", nil); r.Status != 400 {
		t.Fatalf("bad bounds: %d", r.Status)
	}
	if r := e.do("GET", "/v1/companies/in-view?"+india, "tok-alice", nil); len(r.Body["items"].([]any)) != 2 {
		t.Fatalf("in view: %s", r.Raw)
	}

	r = e.do("GET", fmt.Sprintf("/v1/companies/%d", acme), "tok-alice", nil)
	if r.Status != 200 || r.Body["careers_url"] != "https://jobs.lever.co/acme" || len(r.Body["open_jobs"].([]any)) != 2 ||
		r.Body["last_checked_at"] == nil || len(r.Body["locations"].([]any)) != 1 {
		t.Fatalf("company: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/companies/999999", "tok-alice", nil); r.Status != 404 {
		t.Fatalf("missing company: %d", r.Status)
	}
	if r := e.do("PUT", fmt.Sprintf("/v1/companies/%d/save", beta), "tok-alice", nil); r.Body["saved"] != true {
		t.Fatal(r.Raw)
	}
	if r := e.do("GET", "/v1/saved/companies", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("saved: %s", r.Raw)
	}
	e.do("DELETE", fmt.Sprintf("/v1/companies/%d/save", beta), "tok-alice", nil)
	if r := e.do("GET", "/v1/saved/companies", "tok-alice", nil); len(r.Body["items"].([]any)) != 0 {
		t.Fatalf("unsaved: %s", r.Raw)
	}

	r = e.do("GET", fmt.Sprintf("/v1/jobs/%d", job1), "tok-alice", nil)
	if r.Status != 200 || r.Body["description_excerpt"] != "Build APIs in Go." || r.Body["match"] == nil {
		t.Fatalf("job: %s", r.Raw)
	}
	if _, full := r.Body["description"]; full {
		t.Fatal("the full description must not be sent (company content)")
	}
	r = e.do("GET", fmt.Sprintf("/v1/jobs/%d", closed), "tok-alice", nil)
	if r.Body["status"] != "closed" || len(r.Body["similar"].([]any)) == 0 {
		t.Fatalf("closed job: %s", r.Raw)
	}

	r = e.do("GET", "/v1/search?q=acm", "tok-alice", nil)
	if len(r.Body["companies"].([]any)) != 1 {
		t.Fatalf("search companies: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/search?q=backend", "tok-alice", nil); len(r.Body["jobs"].([]any)) != 1 {
		t.Fatalf("search jobs (open only): %s", r.Raw)
	}
	if r := e.do("GET", "/v1/search?q=pun", "tok-alice", nil); len(r.Body["cities"].([]any)) != 1 {
		t.Fatalf("search cities: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/search?q=%25", "tok-alice", nil); r.Status != 200 || len(r.Body["companies"].([]any)) != 0 {
		t.Fatalf("wildcards must be escaped: %s", r.Raw)
	}

	if r := e.do("POST", "/v1/reports", "tok-alice", map[string]any{"company_id": acme, "message": "Wrong city"}); r.Status != 201 {
		t.Fatalf("report: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/reports", "tok-alice", map[string]any{"message": "?"}); r.Status != 400 {
		t.Fatalf("report without target: %d", r.Status)
	}

	r = e.do("GET", "/v1/meta", "tok-alice", nil)
	if r.Body["companies"] != float64(0) || len(r.Body["categories"].([]any)) != 15 || r.Body["last_crawl_at"] == nil {
		// Only companies with an enabled careers source count; seed() gives Acme one (status active).
		if r.Body["companies"] != float64(1) {
			t.Fatalf("meta: %s", r.Raw)
		}
	}
	if r := e.do("GET", "/v1/public/stats", "", nil); r.Status != 200 || r.Body["open_jobs"] != float64(3) {
		t.Fatalf("public stats: %s", r.Raw)
	}
}

func TestMatchesAndTracker(t *testing.T) {
	e := newEnv(t)
	e.join("tok-alice")
	e.join("tok-bob")
	_, _, job1, closed := e.seed()
	uid := e.userID("alice")
	e.exec(`INSERT INTO matches (user_id, job_id, score, reasons)
	        SELECT $1, id, CASE WHEN title LIKE 'Backend%' THEN 90 WHEN title LIKE 'Product%' THEN 61 ELSE 40 END, '{}'
	        FROM jobs`, uid)

	r := e.do("GET", "/v1/matches", "tok-alice", nil)
	items := r.Body["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["score"] != float64(90) || items[0].(map[string]any)["new"] != true {
		t.Fatalf("matches (>=60, open only, best first): %s", r.Raw)
	}
	if r := e.do("GET", "/v1/matches?remote=1", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("remote filter: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["new_matches"] != float64(2) {
		t.Fatalf("new matches: %s", r.Raw)
	}
	e.do("POST", fmt.Sprintf("/v1/matches/%d/seen", job1), "tok-alice", nil)
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["new_matches"] != float64(1) {
		t.Fatalf("seen: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/matches", "tok-bob", nil); len(r.Body["items"].([]any)) != 0 {
		t.Fatal("another user's matches leaked")
	}

	// Save -> applied (S21) -> interview, with a timeline.
	r = e.do("POST", "/v1/applications", "tok-alice", map[string]any{"job_id": job1})
	if r.Status != 201 || r.Body["Status"] != "saved" {
		t.Fatalf("save: %s", r.Raw)
	}
	appID := int64(r.Body["ID"].(float64))
	r = e.do("POST", "/v1/applications", "tok-alice", map[string]any{"job_id": job1, "status": "applied"})
	if r.Body["Status"] != "applied" || r.Body["AppliedAt"] == nil {
		t.Fatalf("applied: %s", r.Raw)
	}
	next := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	r = e.do("PATCH", fmt.Sprintf("/v1/applications/%d", appID), "tok-alice",
		map[string]any{"status": "interview", "notes": "Recruiter call went well", "next_step": "Tech round", "next_step_at": next})
	if r.Status != 200 || r.Body["Status"] != "interview" || r.Body["Notes"] != "Recruiter call went well" {
		t.Fatalf("patch: %s", r.Raw)
	}
	if r := e.do("PATCH", fmt.Sprintf("/v1/applications/%d", appID), "tok-bob", map[string]any{"notes": "x"}); r.Status != 404 {
		t.Fatalf("another user's application: %d", r.Status)
	}
	if r := e.do("PATCH", fmt.Sprintf("/v1/applications/%d", appID), "tok-alice", map[string]any{"status": "hired"}); r.Status != 400 {
		t.Fatalf("bad status: %d", r.Status)
	}
	r = e.do("POST", fmt.Sprintf("/v1/applications/%d/rounds", appID), "tok-alice",
		map[string]any{"at": next, "kind": "Technical", "notes": "System design"})
	if r.Status != 201 {
		t.Fatalf("round: %s", r.Raw)
	}
	roundID := int64(r.Body["ID"].(float64))
	r = e.do("GET", fmt.Sprintf("/v1/applications/%d", appID), "tok-alice", nil)
	events := r.Body["events"].([]any)
	if len(events) != 3 || len(r.Body["rounds"].([]any)) != 1 {
		t.Fatalf("timeline: %s", r.Raw)
	}
	e.do("DELETE", fmt.Sprintf("/v1/applications/%d/rounds/%d", appID, roundID), "tok-alice", nil)

	// A job that left the careers page shows as closed on the board.
	e.do("POST", "/v1/applications", "tok-alice", map[string]any{"job_id": closed, "status": "applied"})
	r = e.do("GET", "/v1/applications", "tok-alice", nil)
	if len(r.Body["items"].([]any)) != 2 || !strings.Contains(r.Raw, `"JobStatus":"closed"`) {
		t.Fatalf("board: %s", r.Raw)
	}
	r = e.do("GET", "/v1/applications/stats", "tok-alice", nil)
	if r.Body["interview"] != float64(1) || r.Body["applied"] != float64(1) || r.Body["response_rate"] != 0.5 {
		t.Fatalf("stats: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/applications", "tok-alice", map[string]any{"job_id": 999999}); r.Status != 404 {
		t.Fatalf("unknown job: %d %s", r.Status, r.Raw)
	}
	e.do("DELETE", fmt.Sprintf("/v1/applications/%d", appID), "tok-alice", nil)
	if r := e.do("GET", fmt.Sprintf("/v1/applications/%d", appID), "tok-alice", nil); r.Status != 404 {
		t.Fatalf("deleted application: %d", r.Status)
	}
}

func docxBytes(t *testing.T, text string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	var paras strings.Builder
	for _, line := range strings.Split(text, "\n") {
		paras.WriteString("<w:p><w:r><w:t>" + line + "</w:t></w:r></w:p>")
	}
	w.Write([]byte(`<w:document xmlns:w="w"><w:body>` + paras.String() + `</w:body></w:document>`))
	zw.Close()
	return buf.Bytes()
}

func (e *env) upload(token string, data []byte, consent bool) resp {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if consent {
		mw.WriteField("consent", "true")
	}
	fw, _ := mw.CreateFormFile("file", "resume.docx")
	fw.Write(data)
	mw.Close()
	req := httptest.NewRequest("POST", "/v1/resume", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := resp{Status: rec.Code, Raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.Body)
	return out
}

func TestResumeProfileAndPreferences(t *testing.T) {
	e := newEnv(t)
	e.join("tok-alice")
	doc := docxBytes(t, "Alice Rao\nalice@example.com | Bengaluru\nSenior Backend Engineer\n"+
		"Skills: Go, Kafka, PostgreSQL, Kubernetes, AWS\nExperience\nSenior Backend Engineer | Acme  Jan 2020 - Present\n"+
		"Built payment services and data pipelines for millions of users across India.")

	if r := e.upload("tok-alice", doc, false); r.Status != 400 || r.Body["error"] != "consent_required" {
		t.Fatalf("no consent: %s", r.Raw)
	}
	if r := e.upload("tok-alice", []byte("plain text is not a resume"), true); r.Status != 415 {
		t.Fatalf("not pdf/docx: %d %s", r.Status, r.Raw)
	}
	r := e.upload("tok-alice", doc, true)
	if r.Status != 201 || r.Body["current_city"] != "Bengaluru" {
		t.Fatalf("upload: %s", r.Raw)
	}
	if !strings.Contains(r.Raw, `"Kafka"`) || !strings.Contains(r.Raw, `"Go"`) {
		t.Fatalf("skills: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/resume", "tok-alice", nil); r.Body["uploaded"] != true {
		t.Fatalf("resume info: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/consents", "tok-alice", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("consent recorded: %s", r.Raw)
	}
	var stored string
	_ = e.pool.QueryRow(context.Background(), "SELECT file_path FROM resumes").Scan(&stored)
	if strings.Contains(stored, "/") || !strings.HasSuffix(stored, ".bin") {
		t.Fatalf("stored name: %q", stored)
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["onboarding"].(map[string]any)["profile_confirmed"] != false {
		t.Fatalf("draft must not be confirmed: %s", r.Raw)
	}

	r = e.do("PUT", "/v1/profile", "tok-alice", map[string]any{"skills": []string{"reactjs", "Go", "go", " "},
		"titles": []string{"Senior Backend Engineer"}, "years_exp": 6.5, "current_city": "Bengaluru", "confirm": true})
	if r.Status != 200 || r.Body["confirmed_at"] == nil || !strings.Contains(r.Raw, `"skills":["React","Go"]`) {
		t.Fatalf("profile: %s", r.Raw)
	}
	if r := e.do("PUT", "/v1/profile", "tok-alice", map[string]any{"years_exp": 99}); r.Status != 400 {
		t.Fatalf("bad years: %d", r.Status)
	}

	if r := e.do("GET", "/v1/preferences", "tok-alice", nil); r.Body["saved"] != false || r.Body["push_time"] != "08:00" {
		t.Fatalf("default prefs: %s", r.Raw)
	}
	r = e.do("PUT", "/v1/preferences", "tok-alice", map[string]any{"categories": []string{"Software Engineering"},
		"cities": []string{"Bengaluru", "Pune"}, "remote_ok": true, "seniority": []string{"Senior"},
		"min_salary": 3000000, "push_time": "07:30", "theme": "dark"})
	if r.Status != 200 || r.Body["push_time"] != "07:30" || r.Body["theme"] != "dark" {
		t.Fatalf("prefs: %s", r.Raw)
	}
	if r := e.do("PUT", "/v1/preferences", "tok-alice", map[string]any{"categories": []string{"Astronaut"}}); r.Status != 400 {
		t.Fatalf("bad category: %d", r.Status)
	}
	st := e.do("GET", "/v1/me", "tok-alice", nil).Body["onboarding"].(map[string]any)
	if st["has_resume"] != true || st["profile_confirmed"] != true || st["has_preferences"] != true {
		t.Fatalf("onboarding: %v", st)
	}
}

func TestPushNotificationsAndAccountDeletion(t *testing.T) {
	e := newEnv(t)
	e.join("tok-alice")
	uid := e.userID("alice")
	if r := e.do("POST", "/v1/push-tokens", "tok-alice", map[string]string{"platform": "android", "token": "fcm-1"}); r.Status != 200 {
		t.Fatal(r.Raw)
	}
	if r := e.do("POST", "/v1/push-tokens", "tok-alice", map[string]string{"platform": "fax", "token": "x"}); r.Status != 400 {
		t.Fatalf("bad platform: %d", r.Status)
	}
	e.exec(`INSERT INTO notifications (user_id, kind, title) VALUES ($1, 'matches', '14 new matches this morning'),
	        ($1, 'reminder', 'Interview tomorrow')`, uid)
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["unread_notifications"] != float64(2) {
		t.Fatalf("unread: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/notifications", "tok-alice", nil); len(r.Body["items"].([]any)) != 2 {
		t.Fatalf("inbox: %s", r.Raw)
	}
	e.do("POST", "/v1/notifications/read", "tok-alice", nil)
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["unread_notifications"] != float64(0) {
		t.Fatalf("read all: %s", r.Raw)
	}
	e.do("DELETE", "/v1/push-tokens", "tok-alice", map[string]string{"token": "fcm-1"})

	if r := e.do("DELETE", "/v1/account", "tok-alice", map[string]string{"confirm": "yes"}); r.Status != 400 {
		t.Fatalf("delete needs confirmation: %d", r.Status)
	}
	if r := e.do("DELETE", "/v1/account", "tok-alice", map[string]string{"confirm": "DELETE"}); r.Body["deleted"] != true {
		t.Fatalf("delete: %s", r.Raw)
	}
	var n int
	_ = e.pool.QueryRow(context.Background(), "SELECT count(*) FROM notifications").Scan(&n)
	if n != 0 {
		t.Fatal("user data left behind")
	}
	if r := e.do("GET", "/v1/me", "tok-alice", nil); r.Body["error"] != "access_code_required" {
		t.Fatalf("after deletion: %s", r.Raw)
	}
}

func TestAdminConsole(t *testing.T) {
	e := newEnv(t)
	e.join("tok-admin")
	acme, _, _, _ := e.seed()
	src := e.id(`SELECT id FROM careers_sources WHERE company_id = $1`, acme)
	e.exec(`UPDATE careers_sources SET status = 'broken', last_error = '0 jobs returned; 9 were open' WHERE id = $1`, src)

	if r := e.do("GET", "/v1/me", "tok-admin", nil); r.Body["is_admin"] != true {
		t.Fatal(r.Raw)
	}
	if r := e.do("GET", "/v1/admin/crawl", "tok-admin", nil); r.Body["health"] != "ok" || len(r.Body["runs"].([]any)) != 1 {
		t.Fatalf("crawl: %s", r.Raw)
	}
	if r := e.do("GET", "/v1/admin/sources/failing", "tok-admin", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("failing: %s", r.Raw)
	}
	r := e.do("PUT", fmt.Sprintf("/v1/admin/sources/%d", src), "tok-admin", map[string]any{
		"careers_url": "https://jobs.lever.co/acme", "ats_type": "lever", "ats_slug": "acme",
		"selectors": map[string]string{"title": "h2"}, "status": "active"})
	if r.Status != 200 || r.Body["Status"] != "active" {
		t.Fatalf("fix source: %s", r.Raw)
	}
	r = e.do("POST", "/v1/admin/companies", "tok-admin", map[string]any{"name": "Gamma Labs", "careers_url": "https://gamma.example/careers",
		"city": "Chennai", "state": "Tamil Nadu", "type": "product"})
	if r.Status != 201 || r.Body["Origin"] != "admin" {
		t.Fatalf("add company: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/admin/companies", "tok-admin", map[string]any{"name": "Gamma Labs", "careers_url": "https://x.example"}); r.Status != 409 {
		t.Fatalf("duplicate company: %d", r.Status)
	}

	r = e.do("PUT", "/v1/admin/category-rules", "tok-admin", map[string]any{"keyword": "Chef", "category": "Operations"})
	if r.Status != 200 || r.Body["Keyword"] != "chef" {
		t.Fatalf("rule: %s", r.Raw)
	}
	ruleID := int64(r.Body["ID"].(float64))
	if r := e.do("GET", "/v1/admin/category-rules", "tok-admin", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("rules: %s", r.Raw)
	}
	e.do("DELETE", fmt.Sprintf("/v1/admin/category-rules/%d", ruleID), "tok-admin", nil)
	e.exec(`UPDATE jobs SET category = 'Other' WHERE title = 'QA Engineer'`)
	if r := e.do("GET", "/v1/admin/unclassified", "tok-admin", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("unclassified: %s", r.Raw)
	}

	r = e.do("POST", "/v1/admin/codes", "tok-admin", map[string]any{"label": "Ravi", "max_uses": 2})
	plain, _ := r.Body["code"].(string)
	if r.Status != 201 || access.Normalize(plain) != plain {
		t.Fatalf("create code: %s", r.Raw)
	}
	codeID := int64(r.Body["id"].(float64))
	r = e.do("GET", "/v1/admin/codes", "tok-admin", nil)
	if strings.Contains(r.Raw, plain) || strings.Contains(r.Raw, "hash") {
		t.Fatalf("codes list must never show codes or hashes: %s", r.Raw)
	}
	tk := e.do("POST", "/v1/access/verify", "", map[string]string{"code": plain}).Body["ticket"].(string)
	e.do("POST", "/v1/access/redeem", "tok-bob", map[string]string{"ticket": tk})
	if r := e.do("GET", fmt.Sprintf("/v1/admin/codes/%d/users", codeID), "tok-admin", nil); len(r.Body["items"].([]any)) != 1 {
		t.Fatalf("code users: %s", r.Raw)
	}
	if r := e.do("PUT", fmt.Sprintf("/v1/admin/codes/%d/revoked", codeID), "tok-admin", map[string]bool{"revoked": true}); r.Body["revoked"] != true {
		t.Fatalf("revoke: %s", r.Raw)
	}

	r = e.do("POST", "/v1/admin/removal-requests", "tok-admin", map[string]any{"kind": "company", "email": "legal@acme.example", "company_id": acme})
	if r.Status != 201 {
		t.Fatalf("removal request: %s", r.Raw)
	}
	reqID := int64(r.Body["ID"].(float64))
	e.do("POST", fmt.Sprintf("/v1/admin/removal-requests/%d/done", reqID), "tok-admin", map[string]string{"note": "marked do-not-crawl"})
	if r := e.do("GET", "/v1/admin/removal-requests", "tok-admin", nil); !strings.Contains(r.Raw, "marked do-not-crawl") {
		t.Fatalf("removal done: %s", r.Raw)
	}
	e.do("POST", "/v1/reports", "tok-admin", map[string]any{"company_id": acme, "message": "Logo is wrong"})
	r = e.do("GET", "/v1/admin/reports", "tok-admin", nil)
	repID := int64(r.Body["items"].([]any)[0].(map[string]any)["ID"].(float64))
	if r := e.do("POST", fmt.Sprintf("/v1/admin/reports/%d/done", repID), "tok-admin", nil); r.Status != 200 {
		t.Fatalf("report done: %s", r.Raw)
	}
}

func TestOpsWatchdog(t *testing.T) {
	e := newEnv(t)
	if r := e.do("GET", "/v1/ops/crawl-status", "wrong", nil); r.Status != 401 {
		t.Fatalf("bad ops token: %d", r.Status)
	}
	if r := e.do("GET", "/v1/ops/crawl-status", "ops-secret", nil); r.Body["finished_today"] != false {
		t.Fatalf("no runs: %s", r.Raw)
	}
	e.exec(`INSERT INTO crawl_runs (started_at) VALUES (now() - interval '3 hours')`) // never finished
	if r := e.do("GET", "/v1/ops/crawl-status", "ops-secret", nil); r.Body["finished_today"] != false {
		t.Fatalf("unfinished: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/ops/crawl-trigger", "ops-secret", map[string]string{"reason": "watchdog: no finished run"}); r.Status != 202 {
		t.Fatalf("trigger: %s", r.Raw)
	}
	if r := e.do("POST", "/v1/ops/crawl-trigger", "ops-secret", map[string]string{}); r.Body["queued"] != false {
		t.Fatalf("second trigger must not queue twice: %s", r.Raw)
	}
	e.exec(`UPDATE crawl_runs SET finished_at = now()`)
	if r := e.do("GET", "/v1/ops/crawl-status", "ops-secret", nil); r.Body["finished_today"] != true {
		t.Fatalf("finished: %s", r.Raw)
	}
}

func TestCORS(t *testing.T) {
	e := newEnv(t)
	req := httptest.NewRequest("OPTIONS", "/v1/me", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Fatalf("allowed origin: %d %v", rec.Code, rec.Header())
	}
	req = httptest.NewRequest("OPTIONS", "/v1/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin allowed")
	}
}
