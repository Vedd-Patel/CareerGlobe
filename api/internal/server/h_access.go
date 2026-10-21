package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/classify"
)

// codeErrors maps code-check failures to what the access screen (S04) shows.
var codeErrors = map[error]*apiError{
	access.ErrInvalidCode:     {http.StatusBadRequest, "invalid_code", "That code is not valid."},
	access.ErrCodeRevoked:     {http.StatusForbidden, "code_revoked", "This code has been revoked."},
	access.ErrCodeExpired:     {http.StatusForbidden, "code_expired", "This code has expired."},
	access.ErrCodeUsedUp:      {http.StatusForbidden, "code_used_up", "This code has already been used."},
	access.ErrTooManyAttempts: {http.StatusTooManyRequests, "too_many_attempts", "Too many tries. Wait 15 minutes and try again."},
	access.ErrTicketUsed:      {http.StatusConflict, "ticket_used", "Enter your access code again."},
	access.ErrBadTicket:       {http.StatusBadRequest, "invalid_ticket", "Enter your access code again."},
	access.ErrTicketExpired:   {http.StatusBadRequest, "ticket_expired", "That took too long. Enter your access code again."},
}

func codeError(err error) error {
	for e, ae := range codeErrors {
		if errors.Is(err, e) {
			return ae
		}
	}
	return err
}

// POST /v1/access/verify {code, device_id} -> {ticket, expires_at}. Public.
func (s *Server) verifyCode(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code     string `json:"code"`
		DeviceID string `json:"device_id"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	device := strings.TrimSpace(in.DeviceID)
	if device == "" {
		device = r.Header.Get("X-Device-Id")
	}
	if len(device) > 128 {
		device = device[:128]
	}
	ticket, expires, err := s.cfg.Access.Verify(r.Context(), in.Code, s.clientIP(r), device)
	if err != nil {
		return codeError(err)
	}
	return ok(w, map[string]any{"ticket": ticket, "expires_at": expires.UTC().Format(time.RFC3339)})
}

// POST /v1/access/redeem {ticket} with a Firebase token -> the linked user.
func (s *Server) redeem(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Ticket string `json:"ticket"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	id := identity(r)
	u, err := s.cfg.Access.Redeem(r.Context(), in.Ticket, id.UID, id.Name, id.Email)
	if err != nil {
		return codeError(err)
	}
	return ok(w, map[string]any{"user_id": u.ID, "created_at": u.CreatedAt})
}

// GET /v1/me: who am I, am I an admin, and where am I in onboarding (S08-S13).
func (s *Server) me(w http.ResponseWriter, r *http.Request) error {
	acc := account(r)
	u, err := s.q.GetUser(r.Context(), acc.UserID)
	if err != nil {
		return err
	}
	st, err := s.q.OnboardingState(r.Context(), acc.UserID)
	if err != nil {
		return err
	}
	unread, err := s.q.CountUnreadNotifications(r.Context(), acc.UserID)
	if err != nil {
		return err
	}
	newMatches, err := s.q.CountNewMatches(r.Context(), acc.UserID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{
		"user_id":   u.ID,
		"name":      u.Name,
		"email":     u.Email,
		"is_admin":  s.cfg.Admins[acc.UID],
		"joined_at": u.CreatedAt,
		"onboarding": map[string]bool{
			"has_resume":        st.HasResume,
			"profile_confirmed": st.ProfileConfirmed,
			"has_preferences":   st.HasPreferences,
		},
		"unread_notifications": unread,
		"new_matches":          newMatches,
	})
}

// GET /v1/meta: lists the apps need (categories, cities, seniority) and when
// the data was last refreshed ("Updated today 6:00 AM").
func (s *Server) meta(w http.ResponseWriter, r *http.Request) error {
	stats, err := s.q.PublicStats(r.Context())
	if err != nil {
		return err
	}
	cities, err := s.q.CityList(r.Context())
	if err != nil {
		return err
	}
	type city struct {
		City      string  `json:"city"`
		State     *string `json:"state"`
		Companies int32   `json:"companies"`
	}
	out := make([]city, 0, len(cities))
	for _, c := range cities {
		if c.City != nil {
			out = append(out, city{*c.City, c.State, c.Companies})
		}
	}
	return ok(w, map[string]any{
		"categories":     classify.Categories,
		"seniority":      classify.SeniorityBands,
		"cities":         out,
		"companies":      stats.Companies,
		"open_jobs":      stats.OpenJobs,
		"last_crawl_at":  stats.LastCrawlAt,
		"policy_version": s.cfg.PolicyVersion,
	})
}

// GET /v1/public/stats: the landing page's live counters (S01). Aggregates only.
func (s *Server) publicStats(w http.ResponseWriter, r *http.Request) error {
	stats, err := s.q.PublicStats(r.Context())
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	writeJSON(w, http.StatusOK, map[string]any{
		"companies": stats.Companies, "open_jobs": stats.OpenJobs, "last_crawl_at": stats.LastCrawlAt,
	})
	return nil
}
