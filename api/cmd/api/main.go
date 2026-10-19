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

	// The server uses its own restricted DB user; local dev falls back to DATABASE_URL.
	dsn := os.Getenv("API_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return errors.New("API_DATABASE_URL or DATABASE_URL must be set")
	}
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// pgx errors can echo the connection string; never log the DSN itself.
		return errors.New("invalid database configuration")
	}
	defer pool.Close()

	srv := &http.Server{
		// Local only: the Cloudflare Tunnel connects to it on the server.
		Addr:              "127.0.0.1:" + port,
		Handler:           server.New(pool),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
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
