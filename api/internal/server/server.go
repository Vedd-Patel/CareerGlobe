// Package server holds the HTTP routes of the CareerGlobe API.
//
// Who can call what:
//   - public: GET /healthz, GET /v1/public/stats (landing page counters, no
//     user data), POST /v1/access/verify (rate limited)
//   - signed in with Firebase, no code yet: POST /v1/access/redeem
//   - members (Firebase token AND a non-revoked access code): everything else under /v1
//   - admins (member + UID on ADMIN_FIREBASE_UIDS): /v1/admin/...
//   - the GitHub watchdog (OPS_TOKEN): /v1/ops/...
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/auth"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/resume"
)

// Pinger is the part of the database pool the health check needs.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Config is everything the handlers depend on.
type Config struct {
	DB            Pinger
	Pool          *pgxpool.Pool
	Access        *access.Service
	Verifier      auth.Verifier
	Resumes       *resume.Service
	Admins        map[string]bool
	OpsToken      string
	Origins       []string
	IPHeader      string
	PolicyVersion string
}

// Server is the API.
type Server struct {
	cfg Config
	q   *db.Queries
}

// New returns the API's root handler.
func New(cfg Config) http.Handler {
	s := &Server{cfg: cfg}
	if cfg.Pool != nil {
		s.q = db.New(cfg.Pool)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	if s.q != nil {
		s.routes(mux)
	}
	return s.logged(s.cors(mux))
}

func (s *Server) routes(mux *http.ServeMux) {
	public := func(pattern string, h handler) { mux.Handle(pattern, h) }
	member := func(pattern string, h handler) { mux.Handle(pattern, s.member(h)) }
	admin := func(pattern string, h handler) { mux.Handle(pattern, s.admin(h)) }

	public("GET /v1/public/stats", s.publicStats)
	public("POST /v1/access/verify", s.verifyCode)
	mux.Handle("POST /v1/access/redeem", s.identified(handler(s.redeem)))

	member("GET /v1/me", s.me)
	member("GET /v1/meta", s.meta)

	member("GET /v1/globe", s.globe)
	member("GET /v1/companies/in-view", s.companiesInView)
	member("GET /v1/companies/{id}", s.company)
	member("PUT /v1/companies/{id}/save", s.saveCompany)
	member("DELETE /v1/companies/{id}/save", s.unsaveCompany)
	member("GET /v1/jobs/{id}", s.job)
	member("GET /v1/search", s.search)
	member("POST /v1/reports", s.report)

	member("GET /v1/matches", s.matches)
	member("POST /v1/matches/{job_id}/seen", s.matchSeen)

	member("GET /v1/applications", s.applications)
	member("GET /v1/applications/stats", s.trackerStats)
	member("POST /v1/applications", s.createApplication)
	member("GET /v1/applications/{id}", s.application)
	member("PATCH /v1/applications/{id}", s.updateApplication)
	member("DELETE /v1/applications/{id}", s.deleteApplication)
	member("POST /v1/applications/{id}/rounds", s.addRound)
	member("DELETE /v1/applications/{id}/rounds/{round_id}", s.deleteRound)

	member("GET /v1/saved/companies", s.savedCompanies)

	member("GET /v1/profile", s.profile)
	member("PUT /v1/profile", s.updateProfile)
	member("GET /v1/preferences", s.preferences)
	member("PUT /v1/preferences", s.updatePreferences)
	member("GET /v1/resume", s.resumeInfo)
	member("POST /v1/resume", s.uploadResume)
	member("GET /v1/consents", s.consents)
	member("POST /v1/push-tokens", s.addPushToken)
	member("DELETE /v1/push-tokens", s.deletePushToken)
	member("GET /v1/notifications", s.notifications)
	member("POST /v1/notifications/read", s.readNotifications)
	member("DELETE /v1/account", s.deleteAccount)

	admin("GET /v1/admin/crawl", s.adminCrawl)
	admin("GET /v1/admin/sources/failing", s.adminFailingSources)
	admin("PUT /v1/admin/sources/{id}", s.adminUpdateSource)
	admin("POST /v1/admin/companies", s.adminCreateCompany)
	admin("GET /v1/admin/category-rules", s.adminCategoryRules)
	admin("PUT /v1/admin/category-rules", s.adminUpsertCategoryRule)
	admin("DELETE /v1/admin/category-rules/{id}", s.adminDeleteCategoryRule)
	admin("GET /v1/admin/unclassified", s.adminUnclassified)
	admin("GET /v1/admin/codes", s.adminCodes)
	admin("POST /v1/admin/codes", s.adminCreateCode)
	admin("PUT /v1/admin/codes/{id}/revoked", s.adminRevokeCode)
	admin("GET /v1/admin/codes/{id}/users", s.adminCodeUsers)
	admin("GET /v1/admin/removal-requests", s.adminRemovalRequests)
	admin("POST /v1/admin/removal-requests", s.adminCreateRemovalRequest)
	admin("POST /v1/admin/removal-requests/{id}/done", s.adminRemovalDone)
	admin("GET /v1/admin/reports", s.adminReports)
	admin("POST /v1/admin/reports/{id}/done", s.adminReportDone)

	mux.Handle("GET /v1/ops/crawl-status", s.ops(handler(s.opsCrawlStatus)))
	mux.Handle("POST /v1/ops/crawl-trigger", s.ops(handler(s.opsCrawlTrigger)))
}

// healthz answers 200 when the database responds, 503 otherwise.
// It is public (uptime checks call it) and reveals no details.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.cfg.DB.Ping(ctx); err != nil {
		slog.Error("healthz: database ping failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
