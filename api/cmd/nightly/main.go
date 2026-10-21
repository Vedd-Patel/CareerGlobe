// Command nightly runs the jobs that follow each crawl.
//
//	nightly run       classify new jobs, score them for every user, add saved-company
//	                  alerts and tracker reminders (after the crawl, before 07:30 IST)
//	nightly classify  only classify
//	nightly match     only score
//	nightly push      send the morning push to users whose time has come (every 15 min)
//	nightly codes     create an access code: nightly codes -label "Ravi" [-uses 1] [-days 30]
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/config"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/nightly"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/push"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nightly run|classify|match|push|codes")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		slog.Error("nightly failed", "step", os.Args[1], "err", err)
		os.Exit(1)
	}
}

func run(step string, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer pool.Close()

	switch step {
	case "classify":
		n, err := nightly.Classify(ctx, pool)
		slog.Info("classified", "jobs", n)
		return err
	case "match":
		users, kept, err := nightly.Match(ctx, pool, time.Now())
		slog.Info("matched", "users", users, "matches", kept)
		return err
	case "run":
		return runAll(ctx, pool, cfg)
	case "push":
		var sender push.Sender = &logSender{}
		if cfg.FirebaseProject != "" {
			fcm, err := push.NewFCM(ctx, cfg.FirebaseProject)
			if err != nil {
				return err
			}
			sender = fcm
		}
		res, err := nightly.Push(ctx, pool, sender)
		slog.Info("push", "users", res.Users, "sent", res.Sent, "dead_tokens", res.DeadTokens)
		return err
	case "codes":
		return createCode(ctx, pool, cfg, args)
	}
	return fmt.Errorf("unknown step %q", step)
}

func runAll(ctx context.Context, pool *pgxpool.Pool, cfg config.Config) (err error) {
	ping(cfg.HealthMatchURL, "/start", "")
	defer func() {
		if err != nil {
			ping(cfg.HealthMatchURL, "/fail", err.Error())
		}
	}()
	start := time.Now()
	n, err := nightly.Classify(ctx, pool)
	if err != nil {
		return err
	}
	users, kept, err := nightly.Match(ctx, pool, time.Now())
	if err != nil {
		return err
	}
	// Saved-company alerts cover jobs first seen since tonight's crawl began.
	var since time.Time
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(started_at), now() - interval '1 day') FROM crawl_runs`).Scan(&since); err != nil {
		return err
	}
	alerts, err := nightly.Alerts(ctx, pool, since)
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("classified %d jobs; scored %d users, %d matches kept; %d saved-company alerts, %d reminders; %s",
		n, users, kept, alerts.SavedAlerts, alerts.Reminders, time.Since(start).Round(time.Second))
	slog.Info(summary)
	ping(cfg.HealthMatchURL, "", summary)
	return nil
}

func createCode(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("codes", flag.ContinueOnError)
	label := fs.String("label", "", "who the code is for (required)")
	uses := fs.Int("uses", 1, "how many accounts may use it")
	days := fs.Int("days", 0, "expires after this many days (0: never)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*label) == "" {
		return errors.New("-label is required")
	}
	hasher, err := access.NewHasher(cfg.Pepper)
	if err != nil {
		return err
	}
	var expires *time.Time
	if *days > 0 {
		t := time.Now().AddDate(0, 0, *days)
		expires = &t
	}
	code, row, err := access.NewService(pool, hasher).CreateCode(ctx, *label, int32(*uses), expires)
	if err != nil {
		return err
	}
	fmt.Printf("%s\t(id %d, %s, max uses %d)\nCopy it now: only its hash is stored.\n", code, row.ID, row.Label, row.MaxUses)
	return nil
}

// ping reports to healthchecks.io when the URL is set; failures are only logged.
func ping(url, suffix, body string) {
	if url == "" {
		return
	}
	if len(body) > 10_000 {
		body = body[:10_000]
	}
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Post(strings.TrimRight(url, "/")+suffix, "text/plain", bytes.NewBufferString(body))
	if err != nil {
		slog.Warn("heartbeat failed", "err", "request error")
		return
	}
	resp.Body.Close()
}

// logSender stands in for FCM before Firebase is set up: it logs instead of sending.
type logSender struct{}

func (logSender) Send(_ context.Context, m push.Message) error {
	slog.Info("push (not sent: FIREBASE_PROJECT_ID not set)", "title", m.Title)
	return nil
}
