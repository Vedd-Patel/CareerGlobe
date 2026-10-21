package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/classify"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
)

// GET /v1/admin/crawl: last nights' crawl_runs and the code-guessing alarm (S30).
func (s *Server) adminCrawl(w http.ResponseWriter, r *http.Request) error {
	runs, err := s.q.LatestCrawlRuns(r.Context(), int32(queryInt(r, "limit", 14, 1, 90)))
	if err != nil {
		return err
	}
	failedGuesses, err := s.q.CountAllFailedAttempts(r.Context(), time.Now().Add(-time.Hour))
	if err != nil {
		return err
	}
	var health string
	switch {
	case len(runs) == 0 || runs[0].StartedAt.Before(time.Now().Add(-30*time.Hour)):
		health = "missing"
	case runs[0].FinishedAt == nil:
		health = "running_or_stopped"
	case runs[0].CompaniesOk+runs[0].CompaniesFailed > 0 &&
		float64(runs[0].CompaniesFailed)/float64(runs[0].CompaniesOk+runs[0].CompaniesFailed) > 0.10:
		health = "too_many_failures"
	default:
		health = "ok"
	}
	return ok(w, map[string]any{"runs": runs, "health": health, "failed_code_guesses_last_hour": failedGuesses})
}

func (s *Server) adminFailingSources(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.FailingSources(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

var sourceStatuses = []string{"active", "broken", "blocked", "review", "disabled"}

// PUT /v1/admin/sources/{id}: fix a failing company ("fix selectors" form).
func (s *Server) adminUpdateSource(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		CareersURL string          `json:"careers_url"`
		AtsType    *string         `json:"ats_type"`
		AtsSlug    *string         `json:"ats_slug"`
		Selectors  json.RawMessage `json:"selectors"`
		Status     string          `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !strings.HasPrefix(in.CareersURL, "https://") && !strings.HasPrefix(in.CareersURL, "http://") {
		return badRequest("careers_url must be an http(s) URL.")
	}
	if !slices.Contains(sourceStatuses, in.Status) {
		return badRequest("Unknown status.")
	}
	var sel []byte
	if len(in.Selectors) > 0 && string(in.Selectors) != "null" {
		if !json.Valid(in.Selectors) {
			return badRequest("selectors must be JSON.")
		}
		sel = in.Selectors
	}
	row, err := s.q.UpdateSource(r.Context(), db.UpdateSourceParams{
		CareersUrl: in.CareersURL, AtsType: in.AtsType, AtsSlug: in.AtsSlug, Selectors: sel, Status: in.Status, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	return ok(w, row)
}

var reSlug = regexp.MustCompile(`[^a-z0-9]+`)

// POST /v1/admin/companies {name, website, careers_url, city, state, type}: add
// a company from the console. The nightly crawl detects its ATS from the URL;
// add it to seeds/companies.csv too so the directory stays in Git.
func (s *Server) adminCreateCompany(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name       string  `json:"name"`
		Website    *string `json:"website"`
		CareersURL string  `json:"careers_url"`
		City       *string `json:"city"`
		State      *string `json:"state"`
		Type       *string `json:"type"`
		AtsType    *string `json:"ats_type"`
		AtsSlug    *string `json:"ats_slug"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || !strings.HasPrefix(in.CareersURL, "http") {
		return badRequest("name and careers_url are required.")
	}
	if in.Type != nil && !slices.Contains([]string{"product", "service", "both"}, *in.Type) {
		return badRequest("type must be product, service or both.")
	}
	slug := strings.Trim(reSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	var company db.Company
	err := pgx.BeginFunc(r.Context(), s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var err error
		company, err = q.AdminCreateCompany(r.Context(), db.AdminCreateCompanyParams{
			Name: name, Slug: slug, Website: in.Website, Type: in.Type})
		if err != nil {
			return err
		}
		if in.City != nil || in.State != nil {
			if err := q.AdminAddLocation(r.Context(), db.AdminAddLocationParams{
				CompanyID: company.ID, City: in.City, State: in.State}); err != nil {
				return err
			}
		}
		_, err = q.AdminAddSource(r.Context(), db.AdminAddSourceParams{
			CompanyID: company.ID, CareersUrl: in.CareersURL, AtsType: in.AtsType, AtsSlug: in.AtsSlug})
		return err
	})
	if isUnique(err) {
		return errStatus(http.StatusConflict, "exists", "A company with this name already exists.")
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, company)
	return nil
}

func (s *Server) adminCategoryRules(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListCategoryRules(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows, "categories": classify.Categories})
}

func (s *Server) adminUpsertCategoryRule(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Keyword  string `json:"keyword"`
		Category string `json:"category"`
		Priority int32  `json:"priority"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	kw := strings.ToLower(strings.TrimSpace(in.Keyword))
	if kw == "" || len(kw) > 60 || !slices.Contains(classify.Categories, in.Category) {
		return badRequest("A rule needs a keyword and a known category.")
	}
	if in.Priority == 0 {
		in.Priority = 5 // admin rules beat the defaults
	}
	row, err := s.q.UpsertCategoryRule(r.Context(), db.UpsertCategoryRuleParams{Keyword: kw, Category: in.Category, Priority: in.Priority})
	if err != nil {
		return err
	}
	return ok(w, row)
}

func (s *Server) adminDeleteCategoryRule(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.q.DeleteCategoryRule(r.Context(), id); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) adminUnclassified(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.UnclassifiedTitles(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// GET /v1/admin/codes: codes with their use counts (never the codes themselves).
func (s *Server) adminCodes(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListCodes(r.Context())
	if err != nil {
		return err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, c := range rows {
		out = append(out, map[string]any{"id": c.ID, "label": c.Label, "max_uses": c.MaxUses, "used_count": c.UsedCount,
			"expires_at": c.ExpiresAt, "revoked": c.Revoked, "created_at": c.CreatedAt, "users": c.UserCount})
	}
	return ok(w, map[string]any{"items": out})
}

// POST /v1/admin/codes {label, max_uses, expires_at}: the plain code is returned once.
func (s *Server) adminCreateCode(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Label     string     `json:"label"`
		MaxUses   int32      `json:"max_uses"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	label := strings.TrimSpace(in.Label)
	if label == "" || len(label) > 100 || in.MaxUses < 0 || in.MaxUses > 1000 {
		return badRequest("A code needs a label (who it is for) and max uses between 1 and 1000.")
	}
	plain, row, err := s.cfg.Access.CreateCode(r.Context(), label, in.MaxUses, in.ExpiresAt)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": row.ID, "code": plain, "label": row.Label,
		"max_uses": row.MaxUses, "expires_at": row.ExpiresAt,
		"note": "Copy this code now: only its hash is stored, it cannot be shown again."})
	return nil
}

// PUT /v1/admin/codes/{id}/revoked {revoked}: users of a revoked code are locked out within a minute.
func (s *Server) adminRevokeCode(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Revoked bool `json:"revoked"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	row, err := s.cfg.Access.SetRevoked(r.Context(), id, in.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"id": row.ID, "revoked": row.Revoked,
		"note": "Signed-in users of this code lose access within " + access.AllowedCacheTTL.String() + "."})
}

func (s *Server) adminCodeUsers(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	rows, err := s.q.UsersByCode(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) adminRemovalRequests(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListRemovalRequests(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// POST /v1/admin/removal-requests {kind, email, user_id?, company_id?, note}: log a
// request that arrived by email (spec: the removal email is the way in).
func (s *Server) adminCreateRemovalRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Kind      string `json:"kind"`
		Email     string `json:"email"`
		UserID    *int64 `json:"user_id"`
		CompanyID *int64 `json:"company_id"`
		Note      string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !slices.Contains([]string{"user_data", "company"}, in.Kind) || !strings.Contains(in.Email, "@") {
		return badRequest("kind (user_data or company) and email are required.")
	}
	row, err := s.q.CreateRemovalRequest(r.Context(), db.CreateRemovalRequestParams{
		Kind: in.Kind, Email: in.Email, UserID: in.UserID, CompanyID: in.CompanyID, Note: in.Note})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, row)
	return nil
}

func (s *Server) adminRemovalDone(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.q.MarkRemovalDone(r.Context(), db.MarkRemovalDoneParams{ID: id, Note: in.Note}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

func (s *Server) adminReports(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListReports(r.Context())
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

func (s *Server) adminReportDone(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.q.MarkReportDone(r.Context(), id); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// GET /v1/ops/crawl-status: for the GitHub watchdog. finished_today is true when
// a run started in the last 24 hours and finished.
func (s *Server) opsCrawlStatus(w http.ResponseWriter, r *http.Request) error {
	run, err := s.q.LastCrawlRun(r.Context())
	if errors.Is(err, pgx.ErrNoRows) {
		return ok(w, map[string]any{"finished_today": false, "last_run": nil})
	}
	if err != nil {
		return err
	}
	recent := run.StartedAt.After(time.Now().Add(-24 * time.Hour))
	return ok(w, map[string]any{
		"finished_today": recent && run.FinishedAt != nil,
		"last_run": map[string]any{"started_at": run.StartedAt, "finished_at": run.FinishedAt,
			"companies_ok": run.CompaniesOk, "companies_failed": run.CompaniesFailed},
	})
}

// POST /v1/ops/crawl-trigger {reason}: ask the server to run the crawl again
// (picked up by the trigger timer). One pending request at a time.
func (s *Server) opsCrawlTrigger(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Reason string `json:"reason"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if p, err := s.q.PendingCrawlTrigger(r.Context()); err == nil {
		return ok(w, map[string]any{"queued": false, "pending_since": p.RequestedAt})
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = "watchdog"
	}
	if len(reason) > 200 {
		reason = reason[:200]
	}
	t, err := s.q.RequestCrawl(r.Context(), reason)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true, "id": t.ID})
	return nil
}
