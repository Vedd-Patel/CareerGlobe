package server

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/classify"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/resume"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/skills"
)

// GET /v1/profile (S28).
func (s *Server) profile(w http.ResponseWriter, r *http.Request) error {
	p, err := s.q.GetProfile(r.Context(), account(r).UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ok(w, map[string]any{"skills": []string{}, "titles": []string{}, "years_exp": nil,
			"current_city": nil, "education": []string{}, "confirmed_at": nil})
	}
	if err != nil {
		return err
	}
	return ok(w, profileJSON(p))
}

func profileJSON(p db.Profile) map[string]any {
	return map[string]any{"skills": p.Skills, "titles": p.Titles, "years_exp": p.YearsExp,
		"current_city": p.CurrentCity, "education": p.Education, "confirmed_at": p.ConfirmedAt, "updated_at": p.UpdatedAt}
}

// PUT /v1/profile {skills, titles, years_exp, current_city, education, confirm}: S09 "Looks good" and S28 edits.
func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Skills      []string `json:"skills"`
		Titles      []string `json:"titles"`
		YearsExp    *float64 `json:"years_exp"`
		CurrentCity *string  `json:"current_city"`
		Education   []string `json:"education"`
		Confirm     bool     `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.YearsExp != nil && (*in.YearsExp < 0 || *in.YearsExp > 60) {
		return badRequest("Years of experience must be between 0 and 60.")
	}
	ext := skills.Default()
	sk := cleanList(in.Skills, 100, 60, ext.Canonical)
	p, err := s.q.UpsertProfile(r.Context(), db.UpsertProfileParams{
		UserID: account(r).UserID, Skills: sk, Titles: cleanList(in.Titles, 10, 80, nil),
		YearsExp: in.YearsExp, CurrentCity: trimmedPtr(in.CurrentCity, 60),
		Education: cleanList(in.Education, 10, 120, nil), Confirm: in.Confirm,
	})
	if err != nil {
		return err
	}
	return ok(w, profileJSON(p))
}

// cleanList trims, dedupes (case-insensitively) and caps a list of short strings.
func cleanList(in []string, maxItems, maxLen int, canon func(string) string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if canon != nil {
			v = canon(v)
		}
		if v == "" || len(v) > maxLen || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
		if len(out) == maxItems {
			break
		}
	}
	return out
}

func trimmedPtr(s *string, max int) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	if len(v) > max {
		v = v[:max]
	}
	return &v
}

// GET /v1/preferences (S10-S13, S29).
func (s *Server) preferences(w http.ResponseWriter, r *http.Request) error {
	p, err := s.q.GetPreferences(r.Context(), account(r).UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ok(w, map[string]any{"categories": []string{}, "cities": []string{}, "anywhere": false,
			"remote_ok": true, "seniority": []string{}, "min_salary": nil, "push_time": "08:00",
			"push_enabled": true, "theme": "system", "saved": false})
	}
	if err != nil {
		return err
	}
	return ok(w, prefsJSON(p))
}

func prefsJSON(p db.Preference) map[string]any {
	return map[string]any{"categories": p.Categories, "cities": p.Cities, "anywhere": p.Anywhere,
		"remote_ok": p.RemoteOk, "seniority": p.Seniority, "min_salary": p.MinSalary,
		"push_time": hhmm(p.PushTime), "push_enabled": p.PushEnabled, "theme": p.Theme, "saved": true}
}

func hhmm(t pgtype.Time) string {
	if !t.Valid {
		return "08:00"
	}
	m := t.Microseconds / 60_000_000
	return time.Date(0, 1, 1, int(m/60), int(m%60), 0, 0, time.UTC).Format("15:04")
}

// PUT /v1/preferences.
func (s *Server) updatePreferences(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Categories  []string `json:"categories"`
		Cities      []string `json:"cities"`
		Anywhere    bool     `json:"anywhere"`
		RemoteOK    *bool    `json:"remote_ok"`
		Seniority   []string `json:"seniority"`
		MinSalary   *int32   `json:"min_salary"`
		PushTime    string   `json:"push_time"`
		PushEnabled *bool    `json:"push_enabled"`
		Theme       string   `json:"theme"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	for _, c := range in.Categories {
		if !slices.Contains(classify.Categories, c) {
			return badRequest("Unknown role category: " + c)
		}
	}
	for _, b := range in.Seniority {
		if !slices.Contains(classify.SeniorityBands, b) {
			return badRequest("Unknown seniority: " + b)
		}
	}
	if in.MinSalary != nil && (*in.MinSalary < 0 || *in.MinSalary > 100_000_000) {
		return badRequest("Minimum salary is out of range.")
	}
	push := pgtype.Time{Microseconds: 8 * 3600 * 1_000_000, Valid: true}
	if in.PushTime != "" {
		t, err := time.Parse("15:04", in.PushTime)
		if err != nil {
			return badRequest("push_time must be HH:MM.")
		}
		push.Microseconds = int64(t.Hour()*3600+t.Minute()*60) * 1_000_000
	}
	theme := in.Theme
	if theme == "" {
		theme = "system"
	}
	if !slices.Contains([]string{"system", "light", "dark"}, theme) {
		return badRequest("theme must be system, light or dark.")
	}
	remote, pushOn := true, true
	if in.RemoteOK != nil {
		remote = *in.RemoteOK
	}
	if in.PushEnabled != nil {
		pushOn = *in.PushEnabled
	}
	p, err := s.q.UpsertPreferences(r.Context(), db.UpsertPreferencesParams{
		UserID: account(r).UserID, Categories: cleanList(in.Categories, 15, 40, nil),
		Cities: cleanList(in.Cities, 30, 60, nil), Anywhere: in.Anywhere, RemoteOk: remote,
		Seniority: cleanList(in.Seniority, 6, 20, nil), MinSalary: in.MinSalary, PushTime: push,
		PushEnabled: pushOn, Theme: theme,
	})
	if err != nil {
		return err
	}
	return ok(w, prefsJSON(p))
}

// GET /v1/resume: the stored file's metadata (never the file).
func (s *Server) resumeInfo(w http.ResponseWriter, r *http.Request) error {
	res, err := s.q.GetResume(r.Context(), account(r).UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ok(w, map[string]any{"uploaded": false, "consent_text": resume.ConsentText})
	}
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"uploaded": true, "original_name": res.OriginalName, "size_bytes": res.SizeBytes,
		"uploaded_at": res.UploadedAt, "parsed_at": res.ParsedAt, "consent_text": resume.ConsentText})
}

// POST /v1/resume (multipart: file, consent=true) -> parsed draft (S08 -> S09).
func (s *Server) uploadResume(w http.ResponseWriter, r *http.Request) error {
	if s.cfg.Resumes == nil {
		return errStatus(http.StatusServiceUnavailable, "uploads_off", "Resume upload is not set up on this server yet.")
	}
	r.Body = http.MaxBytesReader(w, r.Body, resume.MaxUpload+64<<10)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return errStatus(http.StatusRequestEntityTooLarge, "too_large", "The file must be a PDF or DOCX up to 5 MB.")
	}
	defer r.MultipartForm.RemoveAll()
	if r.FormValue("consent") != "true" {
		return errStatus(http.StatusBadRequest, "consent_required", "Tick the consent box to upload your resume.")
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest("Attach a PDF or DOCX file.")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, resume.MaxUpload+1))
	if err != nil {
		return err
	}
	draft, err := s.cfg.Resumes.Upload(r.Context(), account(r).UserID, hdr.Filename, data, resume.Consent{
		Given: true, PolicyVersion: s.cfg.PolicyVersion, IP: s.clientIP(r), UserAgent: r.UserAgent(),
	})
	switch {
	case errors.Is(err, resume.ErrTooLarge):
		return errStatus(http.StatusRequestEntityTooLarge, "too_large", "The file must be a PDF or DOCX up to 5 MB.")
	case errors.Is(err, resume.ErrUnsupported):
		return errStatus(http.StatusUnsupportedMediaType, "unsupported", "Only PDF and DOCX files are accepted.")
	case errors.Is(err, resume.ErrNoText):
		return errStatus(http.StatusUnprocessableEntity, "no_text", "Couldn't read this file — try a text PDF.")
	case err != nil:
		return err
	}
	writeJSON(w, http.StatusCreated, draft)
	return nil
}

// GET /v1/consents: the consents this user gave, with dates (S29).
func (s *Server) consents(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListConsents(r.Context(), account(r).UserID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows, "consent_text": resume.ConsentText})
}

// POST /v1/push-tokens {platform, token}.
func (s *Server) addPushToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Platform string `json:"platform"`
		Token    string `json:"token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if !slices.Contains([]string{"android", "ios", "web"}, in.Platform) || in.Token == "" || len(in.Token) > 4096 {
		return badRequest("platform must be android, ios or web, with a token.")
	}
	if err := s.q.UpsertPushToken(r.Context(), db.UpsertPushTokenParams{
		UserID: account(r).UserID, Platform: in.Platform, FcmToken: in.Token}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// DELETE /v1/push-tokens {token}: on log out.
func (s *Server) deletePushToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if err := s.q.DeletePushToken(r.Context(), db.DeletePushTokenParams{UserID: account(r).UserID, FcmToken: in.Token}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// GET /v1/notifications (S27).
func (s *Server) notifications(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListNotifications(r.Context(), account(r).UserID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// POST /v1/notifications/read {id?}: mark one (or all) as read.
func (s *Server) readNotifications(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		ID *int64 `json:"id"`
	}
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			return err
		}
	}
	if err := s.q.MarkNotificationsRead(r.Context(), db.MarkNotificationsReadParams{UserID: account(r).UserID, ID: in.ID}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// DELETE /v1/account {confirm: "DELETE"}: erase the account, profile, resume
// file, matches and tracker (DPDP erasure; the spec recommends offering it).
// The apps decide whether to show the button; the access code use is not refunded.
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Confirm != "DELETE" {
		return badRequest(`Send {"confirm": "DELETE"} to delete your account and data.`)
	}
	uid := account(r).UserID
	if s.cfg.Resumes != nil {
		if err := s.cfg.Resumes.DeleteFiles(r.Context(), uid); err != nil {
			return err
		}
	}
	if err := s.q.DeleteUser(r.Context(), uid); err != nil { // cascades to every user table
		return err
	}
	s.cfg.Access.Forget(account(r).UID)
	return ok(w, map[string]bool{"deleted": true})
}
