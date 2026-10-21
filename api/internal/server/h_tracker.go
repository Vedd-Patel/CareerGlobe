package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
)

// MatchThreshold: only jobs scoring this or more count as a "match" (spec).
const MatchThreshold = 60

var statuses = []string{"saved", "applied", "interview", "offer", "rejected", "withdrawn"}

// GET /v1/matches?sort=best|newest&category&remote&type&city&min_score&limit&offset (S19).
func (s *Server) matches(w http.ResponseWriter, r *http.Request) error {
	f := filtersFrom(r)
	q := r.URL.Query()
	rows, err := s.q.ListMatches(r.Context(), db.ListMatchesParams{
		UserID: account(r).UserID, MinScore: int32(queryInt(r, "min_score", MatchThreshold, 0, 100)),
		Category: f.Category, RemoteOnly: f.RemoteOnly, CompanyType: f.CompanyType, City: optString(q.Get("city")),
		SortNewest: q.Get("sort") == "newest", MaxRows: int32(queryInt(r, "limit", 30, 1, 100)),
		Skip: int32(queryInt(r, "offset", 0, 0, 10_000)),
	})
	if err != nil {
		return err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, map[string]any{
			"job_id": m.JobID, "title": m.Title, "category": m.Category, "cities": m.Cities, "work_mode": m.WorkMode,
			"posted_at": m.PostedAt, "first_seen": m.FirstSeen, "company_id": m.CompanyID,
			"company_name": m.CompanyName, "logo_url": m.LogoUrl, "score": m.Score,
			"reasons": json.RawMessage(m.Reasons), "new": !m.Seen, "saved": m.Saved,
		})
	}
	return ok(w, map[string]any{"items": items})
}

func (s *Server) matchSeen(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "job_id")
	if err != nil {
		return err
	}
	if err := s.q.MarkMatchSeen(r.Context(), db.MarkMatchSeenParams{UserID: account(r).UserID, JobID: id}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// GET /v1/applications: the tracker board (S23).
func (s *Server) applications(w http.ResponseWriter, r *http.Request) error {
	rows, err := s.q.ListApplications(r.Context(), account(r).UserID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": rows})
}

// GET /v1/applications/stats: counts per stage and response rate (S25).
func (s *Server) trackerStats(w http.ResponseWriter, r *http.Request) error {
	st, err := s.q.TrackerStats(r.Context(), account(r).UserID)
	if err != nil {
		return err
	}
	var rate *float64
	if st.EverApplied > 0 {
		v := float64(st.ReachedInterview) / float64(st.EverApplied)
		rate = &v
	}
	return ok(w, map[string]any{
		"saved": st.Saved, "applied": st.Applied, "interview": st.Interview, "offer": st.Offer,
		"rejected": st.Rejected, "withdrawn": st.Withdrawn, "response_rate": rate,
	})
}

// POST /v1/applications {job_id, status}: save a job or mark it applied (S20, S21).
func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		JobID  int64  `json:"job_id"`
		Status string `json:"status"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Status == "" {
		in.Status = "saved"
	}
	if !slices.Contains(statuses, in.Status) {
		return badRequest("Unknown status.")
	}
	uid := account(r).UserID
	var app db.Application
	err := pgx.BeginFunc(r.Context(), s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		existing, err := q.GetApplicationForJob(r.Context(), db.GetApplicationForJobParams{UserID: uid, JobID: in.JobID})
		if err == nil {
			app = existing
			if existing.Status == in.Status {
				return nil
			}
			return s.changeStatus(r, q, &app, in.Status)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		app, err = q.CreateApplication(r.Context(), db.CreateApplicationParams{UserID: uid, JobID: in.JobID, Status: in.Status})
		if err != nil {
			return err
		}
		return q.AddApplicationEvent(r.Context(), db.AddApplicationEventParams{ApplicationID: app.ID, ToStatus: in.Status})
	})
	if isForeignKey(err) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, app)
	return nil
}

func (s *Server) changeStatus(r *http.Request, q *db.Queries, app *db.Application, status string) error {
	from := app.Status
	updated, err := q.UpdateApplication(r.Context(), db.UpdateApplicationParams{
		Status: status, Notes: app.Notes, NextStep: app.NextStep, NextStepAt: app.NextStepAt,
		DontAsk: app.DontAsk, ID: app.ID, UserID: app.UserID,
	})
	if err != nil {
		return err
	}
	*app = updated
	return q.AddApplicationEvent(r.Context(), db.AddApplicationEventParams{ApplicationID: app.ID, FromStatus: &from, ToStatus: status})
}

// GET /v1/applications/{id}: detail with timeline and interview rounds (S24).
func (s *Server) application(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	app, err := s.q.GetApplication(r.Context(), db.GetApplicationParams{ID: id, UserID: account(r).UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	events, err := s.q.ApplicationEvents(r.Context(), id)
	if err != nil {
		return err
	}
	rounds, err := s.q.InterviewRounds(r.Context(), id)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"application": app, "events": events, "rounds": rounds})
}

// PATCH /v1/applications/{id} {status?, notes?, next_step?, next_step_at?, dont_ask?}.
func (s *Server) updateApplication(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status     *string    `json:"status"`
		Notes      *string    `json:"notes"`
		NextStep   *string    `json:"next_step"`
		NextStepAt *time.Time `json:"next_step_at"`
		ClearNext  bool       `json:"clear_next_step"`
		DontAsk    *bool      `json:"dont_ask"`
		Note       string     `json:"event_note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Status != nil && !slices.Contains(statuses, *in.Status) {
		return badRequest("Unknown status.")
	}
	if in.Notes != nil && len(*in.Notes) > 20_000 {
		return badRequest("Notes are too long.")
	}
	uid := account(r).UserID
	var out db.Application
	err = pgx.BeginFunc(r.Context(), s.cfg.Pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		cur, err := q.GetApplication(r.Context(), db.GetApplicationParams{ID: id, UserID: uid})
		if err != nil {
			return err
		}
		p := db.UpdateApplicationParams{Status: cur.Status, Notes: cur.Notes, NextStep: cur.NextStep,
			NextStepAt: cur.NextStepAt, DontAsk: cur.DontAsk, ID: id, UserID: uid}
		if in.Status != nil {
			p.Status = *in.Status
		}
		if in.Notes != nil {
			p.Notes = *in.Notes
		}
		if in.NextStep != nil {
			p.NextStep = optString(strings.TrimSpace(*in.NextStep))
		}
		if in.NextStepAt != nil {
			p.NextStepAt = in.NextStepAt
		}
		if in.ClearNext {
			p.NextStep, p.NextStepAt = nil, nil
		}
		if in.DontAsk != nil {
			p.DontAsk = *in.DontAsk
		}
		out, err = q.UpdateApplication(r.Context(), p)
		if err != nil {
			return err
		}
		if p.Status != cur.Status {
			from := cur.Status
			return q.AddApplicationEvent(r.Context(), db.AddApplicationEventParams{
				ApplicationID: id, FromStatus: &from, ToStatus: p.Status, Note: optString(in.Note)})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) deleteApplication(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := s.q.DeleteApplication(r.Context(), db.DeleteApplicationParams{ID: id, UserID: account(r).UserID}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}

// POST /v1/applications/{id}/rounds {at, kind, notes}.
func (s *Server) addRound(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		At    time.Time `json:"at"`
		Kind  string    `json:"kind"`
		Notes string    `json:"notes"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	kind := strings.TrimSpace(in.Kind)
	if in.At.IsZero() || kind == "" || len(kind) > 100 || len(in.Notes) > 5000 {
		return badRequest("A round needs a date and a type.")
	}
	if _, err := s.q.GetApplication(r.Context(), db.GetApplicationParams{ID: id, UserID: account(r).UserID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound
		}
		return err
	}
	round, err := s.q.AddInterviewRound(r.Context(), db.AddInterviewRoundParams{ApplicationID: id, At: in.At, Kind: kind, Notes: in.Notes})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, round)
	return nil
}

func (s *Server) deleteRound(w http.ResponseWriter, r *http.Request) error {
	rid, err := pathID(r, "round_id")
	if err != nil {
		return err
	}
	if err := s.q.DeleteInterviewRound(r.Context(), db.DeleteInterviewRoundParams{ID: rid, UserID: account(r).UserID}); err != nil {
		return err
	}
	return ok(w, map[string]bool{"ok": true})
}
