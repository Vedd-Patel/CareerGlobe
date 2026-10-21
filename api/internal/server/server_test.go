package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		db         fakeDB
		wantStatus int
		wantBody   string
	}{
		{"database up", fakeDB{}, http.StatusOK, `{"status":"ok"}`},
		{"database down", fakeDB{errors.New("connection refused")}, http.StatusServiceUnavailable, `{"status":"unavailable"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			New(Config{DB: tt.db}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

func TestHealthzRejectsOtherMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	New(Config{DB: fakeDB{}}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestExcerpt(t *testing.T) {
	desc := "<p>Build <b>APIs</b> &amp; services.</p><ul><li>Go</li><li>Postgres</li></ul>"
	if got := Excerpt(&desc, 100); got != "Build APIs & services.\nGo\nPostgres" {
		t.Fatalf("Excerpt = %q", got)
	}
	long := "<p>" + strings.Repeat("word ", 400) + "</p>"
	if got := Excerpt(&long, 50); len([]rune(got)) > 52 || !strings.HasSuffix(got, "…") {
		t.Fatalf("long excerpt = %q", got)
	}
	if Excerpt(nil, 10) != "" {
		t.Fatal("nil")
	}
}
