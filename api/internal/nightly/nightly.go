// Package nightly runs the jobs after each crawl: classify new jobs, score
// them for every user, and (on its own timer) send the morning push.
package nightly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/classify"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/match"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/push"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/skills"
)

const (
	classifyBatch = 2000
	// StoreFloor: scores below this are not stored (they would never be shown).
	StoreFloor = 30
)

// Classify sets category, seniority and skills on every open job whose text
// changed since it was last classified. Admin rules and synonyms come from the DB.
func Classify(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	q := db.New(pool)
	rules, err := q.CategoryRuleList(ctx)
	if err != nil {
		return 0, err
	}
	extra := make([]classify.Rule, 0, len(rules))
	for _, r := range rules {
		extra = append(extra, classify.Rule{Keyword: r.Keyword, Category: r.Category, Priority: int(r.Priority)})
	}
	syn, err := q.SkillSynonymList(ctx)
	if err != nil {
		return 0, err
	}
	synonyms := map[string]string{}
	for _, s := range syn {
		synonyms[s.Alias] = s.Canonical
	}
	cl, ext := classify.New(extra), skills.New(synonyms)

	total := 0
	for {
		jobs, err := q.JobsToClassify(ctx, classifyBatch)
		if err != nil {
			return total, err
		}
		if len(jobs) == 0 {
			return total, nil
		}
		err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			qt := q.WithTx(tx)
			for _, j := range jobs {
				first, second := cl.Category(j.Title)
				seniority := classify.Seniority(j.Title)
				sk := ext.Extract(j.Title + "\n" + plain(j.Description))
				if sk == nil {
					sk = []string{}
				}
				if err := qt.SetJobClassification(ctx, db.SetJobClassificationParams{
					ID: j.ID, Category: &first, Category2: optional(second), Seniority: &seniority,
					Skills: sk, ClassifiedHash: &j.TextHash,
				}); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += len(jobs)
	}
}

// Match rebuilds every user's matches. Returns users scored and matches kept (>= StoreFloor).
func Match(ctx context.Context, pool *pgxpool.Pool, now time.Time) (int, int, error) {
	q := db.New(pool)
	states, err := q.CityStates(ctx)
	if err != nil {
		return 0, 0, err
	}
	stateOf := map[string]string{}
	for _, s := range states {
		stateOf[strings.ToLower(*s.City)] = *s.State
	}
	lookup := func(city string) string { return stateOf[strings.ToLower(city)] }

	users, err := q.UsersToMatch(ctx)
	if err != nil {
		return 0, 0, err
	}
	kept := 0
	for _, u := range users {
		start := time.Now()
		n, err := matchUser(ctx, pool, q, u, lookup, now)
		if err != nil {
			return 0, kept, fmt.Errorf("user %d: %w", u.ID, err)
		}
		kept += n
		slog.Info("matched", "user", u.ID, "kept", n, "ms", time.Since(start).Milliseconds())
	}
	return len(users), kept, nil
}

func matchUser(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, u db.UsersToMatchRow,
	stateOf func(string) string, now time.Time) (int, error) {
	user := match.User{Skills: u.Skills, Titles: u.Titles, YearsExp: u.YearsExp, Categories: u.Categories,
		Cities: u.Cities, Anywhere: u.Anywhere, RemoteOK: u.RemoteOk, Seniority: u.Seniority}
	if u.CurrentCity != nil {
		user.City = *u.CurrentCity
	}
	jobs, err := q.CandidateJobs(ctx, db.CandidateJobsParams{Categories: u.Categories, Anywhere: u.Anywhere,
		Cities: append(u.Cities, user.City), RemoteOk: u.RemoteOk, MinSalary: u.MinSalary})
	if err != nil {
		return 0, err
	}
	// One transaction per user: their matches are replaced all at once.
	kept := 0
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		qt := q.WithTx(tx)
		for _, j := range jobs {
			score, reasons := match.Score(user, match.Job{Title: j.Title, Category: deref(j.Category),
				Category2: deref(j.Category2), Seniority: deref(j.Seniority), Skills: j.Skills, Cities: j.Cities,
				WorkMode: deref(j.WorkMode), FirstSeen: j.FirstSeen}, stateOf, now)
			if score < StoreFloor {
				continue
			}
			raw, err := json.Marshal(reasons)
			if err != nil {
				return err
			}
			if err := qt.UpsertMatch(ctx, db.UpsertMatchParams{UserID: u.ID, JobID: j.ID, Score: int16(score), Reasons: raw}); err != nil {
				return err
			}
			kept++
		}
		_, err := qt.DeleteStaleMatches(ctx, u.ID)
		return err
	})
	return kept, err
}

// PushResult counts what Push did.
type PushResult struct {
	Users, Sent, DeadTokens int
}

// Push sends the morning "new matches" push to users whose push time has come
// (at most one a day, only when there is a new match >= 60). Runs every 15 minutes.
func Push(ctx context.Context, pool *pgxpool.Pool, sender push.Sender) (PushResult, error) {
	q := db.New(pool)
	var res PushResult
	due, err := q.UsersDueForPush(ctx)
	if err != nil {
		return res, err
	}
	for _, u := range due {
		title := fmt.Sprintf("%d new matches this morning", u.NewMatches)
		if u.NewMatches == 1 {
			title = "1 new match this morning"
		}
		body := "Open CareerGlobe to see the jobs that fit you best."
		if err := q.AddNotification(ctx, db.AddNotificationParams{UserID: u.ID, Kind: "matches", Title: title,
			Body: body, Link: ptr("/matches")}); err != nil {
			return res, err
		}
		tokens, err := q.PushTokensForUser(ctx, u.ID)
		if err != nil {
			return res, err
		}
		for _, t := range tokens {
			err := sender.Send(ctx, push.Message{Token: t.FcmToken, Title: title, Body: body, Link: "/matches"})
			switch {
			case errors.Is(err, push.ErrUnregistered):
				res.DeadTokens++
				if err := q.DeletePushTokenByValue(ctx, t.FcmToken); err != nil {
					return res, err
				}
			case err != nil:
				slog.Warn("push failed", "user", u.ID, "err", err) // the inbox item still shows it
			default:
				res.Sent++
			}
		}
		if err := q.MarkPushed(ctx, u.ID); err != nil {
			return res, err
		}
		if err := q.SetLastPush(ctx, u.ID); err != nil {
			return res, err
		}
		res.Users++
	}
	return res, nil
}

// AlertResult counts what Alerts did.
type AlertResult struct {
	SavedAlerts, Reminders int
}

// Alerts adds inbox items once a night after matching: saved companies that
// posted matching jobs since `since` (tonight's crawl start), and tracker next
// steps due tomorrow.
func Alerts(ctx context.Context, pool *pgxpool.Pool, since time.Time) (AlertResult, error) {
	q := db.New(pool)
	var res AlertResult
	saved, err := q.SavedCompanyNewJobs(ctx, since)
	if err != nil {
		return res, err
	}
	for _, s := range saved {
		title := fmt.Sprintf("%s posted %d new job", s.CompanyName, s.Jobs)
		if s.Jobs != 1 {
			title += "s"
		}
		if err := q.AddNotification(ctx, db.AddNotificationParams{UserID: s.UserID, Kind: "saved_company",
			Title: title, Body: "A company you saved is hiring.", Link: ptr(fmt.Sprintf("/companies/%d", s.CompanyID))}); err != nil {
			return res, err
		}
		res.SavedAlerts++
	}

	steps, err := q.UpcomingNextSteps(ctx)
	if err != nil {
		return res, err
	}
	for _, s := range steps {
		what := "Next step"
		if s.NextStep != nil && *s.NextStep != "" {
			what = *s.NextStep
		}
		if err := q.AddNotification(ctx, db.AddNotificationParams{UserID: s.UserID, Kind: "reminder",
			Title: what + " tomorrow", Body: s.Title + " at " + s.CompanyName,
			Link: ptr(fmt.Sprintf("/tracker/%d", s.ID))}); err != nil {
			return res, err
		}
		res.Reminders++
	}
	return res, nil
}

func plain(htmlText string) string {
	var b strings.Builder
	in := false
	for _, r := range htmlText {
		switch {
		case r == '<':
			in = true
			b.WriteByte(' ')
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.NewReplacer("&amp;", "&", "&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&#39;", "'", "&quot;", `"`).Replace(b.String())
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptr(s string) *string { return &s }
