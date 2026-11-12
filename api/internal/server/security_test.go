package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityHeadersOnEveryAnswer(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Config{DB: fakeDB{}}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	for name, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("Strict-Transport-Security is missing")
	}
}

func TestLimiterRefillsOverTime(t *testing.T) {
	now := time.Unix(1_000, 0)
	l := newLimiter(60, 2) // one a second, two at once
	l.now = func() time.Time { return now }
	for i := range 2 {
		if ok, _ := l.allow("a"); !ok {
			t.Fatalf("request %d refused inside the burst", i+1)
		}
	}
	ok, wait := l.allow("a")
	if ok || wait <= 0 || wait > time.Second {
		t.Fatalf("third request: ok=%v wait=%v, want refused with a wait of up to 1s", ok, wait)
	}
	if ok, _ := l.allow("b"); !ok {
		t.Fatal("another client must have its own budget")
	}
	now = now.Add(time.Second)
	if ok, _ := l.allow("a"); !ok {
		t.Fatal("refused after the wait")
	}
}

func TestLimiterForgetsIdleClients(t *testing.T) {
	now := time.Unix(1_000, 0)
	l := newLimiter(60, 2)
	l.now = func() time.Time { return now }
	l.allow("idle")
	now = now.Add(time.Minute)
	l.sweep(now)
	if len(l.buckets) != 0 {
		t.Fatalf("%d buckets left after the sweep", len(l.buckets))
	}
}

func TestRequestsOverTheBudgetGet429(t *testing.T) {
	h := New(Config{DB: fakeDB{}, RatePerMinute: 4}) // burst of 2
	get := func(path, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = ip + ":4000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for range 2 {
		if rec := get("/v1/me", "203.0.113.7"); rec.Code == http.StatusTooManyRequests {
			t.Fatal("refused inside the burst")
		}
	}
	rec := get("/v1/me", "203.0.113.7")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d, Retry-After = %q; want 429 with a wait", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := get("/v1/me", "203.0.113.8"); rec.Code == http.StatusTooManyRequests {
		t.Error("a different IP was refused")
	}
	for range 5 {
		if rec := get("/healthz", "203.0.113.7"); rec.Code != http.StatusOK {
			t.Fatalf("/healthz = %d; uptime checks must never be limited", rec.Code)
		}
	}
}

func TestPublicURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://careers.example.com/jobs":      true,
		"http://example.co.in":                  true,
		"https://boards.greenhouse.io/groww":    true,
		"":                                      false,
		"ftp://example.com":                     false,
		"javascript:alert(1)":                   false,
		"https://localhost/x":                   false,
		"http://127.0.0.1:8080/healthz":         false,
		"http://169.254.169.254/latest/meta":    false,
		"http://[::1]/":                         false,
		"https://10.0.0.5/":                     false,
		"https://db.internal/":                  false,
		"https://printer.local/":                false,
		"https://user:pass@example.com/":        false,
		"https://intranet/":                     false,
		"https://example.com./careers":          true,
		"http://2130706433/":                    false,
		"https://careers.example.com/a?b=c#top": true,
	} {
		if got := publicURL(raw); got != want {
			t.Errorf("publicURL(%q) = %v, want %v", raw, got, want)
		}
	}
}
