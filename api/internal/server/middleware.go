package server

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/access"
	"github.com/Vedd-Patel/CareerGlobe/api/internal/auth"
)

type ctxKey int

const (
	accountKey ctxKey = iota
	identityKey
)

// recoverer turns panics into 500s and logs one line per request.
func (s *Server) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic", "path", r.URL.Path, "panic", p)
				writeJSON(rec, http.StatusInternalServerError, map[string]string{"error": "internal"})
			}
			// Never log query strings: they can carry search text.
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
				"ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// cors allows the web app's origins (WEB_ORIGINS) to call the API.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(s.cfg.Origins, origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Device-Id")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is Cloudflare's connecting-IP header when present (the API only
// listens on localhost behind the tunnel), else the socket address.
func (s *Server) clientIP(r *http.Request) *netip.Addr {
	if s.cfg.IPHeader != "" {
		if a, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get(s.cfg.IPHeader))); err == nil {
			return &a
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return &a
	}
	return nil
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// identified requires a valid Firebase token (used by /v1/access/redeem, where
// the account is not linked to a code yet).
func (s *Server) identified(next http.Handler) http.Handler {
	return handler(func(w http.ResponseWriter, r *http.Request) error {
		id, err := s.cfg.Verifier.Verify(r.Context(), bearer(r))
		if err != nil {
			return errUnauthorized
		}
		if id.Provider == "password" && !id.EmailVerified {
			return errStatus(http.StatusForbidden, "email_not_verified", "Verify your email address first.")
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
		return nil
	})
}

// member is the check every /v1 endpoint runs: a valid Firebase token AND an
// account linked to a code that is not revoked (spec: Private access, rule 4).
func (s *Server) member(next http.Handler) http.Handler {
	return s.identified(handler(func(w http.ResponseWriter, r *http.Request) error {
		id := identity(r)
		acc, err := s.cfg.Access.Allowed(r.Context(), id.UID)
		switch {
		case errors.Is(err, access.ErrNoAccess):
			return errStatus(http.StatusForbidden, "access_code_required", "Enter your access code to continue.")
		case errors.Is(err, access.ErrAccessRemoved):
			return errStatus(http.StatusForbidden, "access_removed", "Your access has been removed.")
		case err != nil:
			return err
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountKey, acc)))
		return nil
	}))
}

// admin additionally requires a Firebase UID on the ADMIN_FIREBASE_UIDS list.
func (s *Server) admin(next http.Handler) http.Handler {
	return s.member(handler(func(w http.ResponseWriter, r *http.Request) error {
		if !s.cfg.Admins[account(r).UID] {
			return errStatus(http.StatusForbidden, "not_admin", "Admins only.")
		}
		next.ServeHTTP(w, r)
		return nil
	}))
}

// ops is for the GitHub watchdog: a shared bearer token (OPS_TOKEN), no Firebase.
func (s *Server) ops(next http.Handler) http.Handler {
	return handler(func(w http.ResponseWriter, r *http.Request) error {
		tok := bearer(r)
		if s.cfg.OpsToken == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.OpsToken)) != 1 {
			return errUnauthorized
		}
		next.ServeHTTP(w, r)
		return nil
	})
}

func identity(r *http.Request) auth.Identity {
	id, _ := r.Context().Value(identityKey).(auth.Identity)
	return id
}

func account(r *http.Request) access.Account {
	acc, _ := r.Context().Value(accountKey).(access.Account)
	return acc
}
