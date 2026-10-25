// Command api runs the CareerGlobe HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/auth"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/config"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/resume"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		// pgx errors can echo the connection string; never log the DSN itself.
		return errors.New("invalid database configuration")
	}
	defer pool.Close()

	hasher, err := access.NewHasher(cfg.Pepper)
	if err != nil {
		return err
	}

	// Until Firebase is set up, every signed-in endpoint answers 401; /healthz still works.
	var verifier auth.Verifier = auth.Static{}
	if cfg.FirebaseProject != "" {
		fb, err := auth.NewFirebase(ctx, cfg.FirebaseProject)
		if err != nil {
			return err
		}
		verifier = fb
	} else if os.Getenv("DEV_AUTH") == "1" {
		slog.Warn("DEV_AUTH=1: accepting dev:<uid>:<email> tokens (local development only)")
		verifier = auth.Dev{}
	} else {
		slog.Warn("FIREBASE_PROJECT_ID is not set: sign-in is off")
	}

	var resumes *resume.Service
	if cfg.ResumeKey != nil {
		vault, err := resume.NewVault(cfg.ResumeDir, cfg.ResumeKey)
		if err != nil {
			return err
		}
		resumes = resume.NewService(pool, vault)
	} else {
		slog.Warn("RESUME_ENCRYPTION_KEY is not set: resume upload is off")
	}

	srv := &http.Server{
		// Local only: the Cloudflare Tunnel connects to it on the server.
		Addr: "127.0.0.1:" + cfg.Port,
		Handler: server.New(server.Config{
			DB: pool, Pool: pool, Access: access.NewService(pool, hasher), Verifier: verifier, Resumes: resumes,
			Admins: cfg.AdminUIDs, OpsToken: cfg.OpsToken, Origins: cfg.WebOrigins, IPHeader: cfg.TrustProxyHeader,
			PolicyVersion: cfg.PolicyVersion,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	slog.Info("api shutting down")
	return srv.Shutdown(shutdownCtx)
}
