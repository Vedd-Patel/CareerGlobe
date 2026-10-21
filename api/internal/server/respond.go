package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgconn"
)

// apiError is an error with an HTTP status and a stable code the apps switch on.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Code }

func errStatus(status int, code, msg string) error { return &apiError{status, code, msg} }

var (
	errNotFound     = errStatus(http.StatusNotFound, "not_found", "Not found.")
	errUnauthorized = errStatus(http.StatusUnauthorized, "unauthorized", "Sign in again.")
)

func badRequest(msg string) error { return errStatus(http.StatusBadRequest, "bad_request", msg) }

// handler is an http.HandlerFunc that returns an error instead of writing it.
type handler func(w http.ResponseWriter, r *http.Request) error

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := h(w, r)
	if err == nil {
		return
	}
	var ae *apiError
	if errors.As(err, &ae) {
		writeJSON(w, ae.Status, map[string]string{"error": ae.Code, "message": ae.Message})
		return
	}
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal", "message": "Something went wrong."})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ok(w http.ResponseWriter, v any) error {
	writeJSON(w, http.StatusOK, v)
	return nil
}

// decode reads a JSON body of at most 64 kB into v, rejecting unknown fields.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("Invalid JSON body.")
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

func queryInt(r *http.Request, name string, def, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return def
	}
	return clamp(v, min, max)
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func queryBool(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	return v == "1" || v == "true"
}

// optString is nil for "" (a filter that is off).
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isForeignKey is true for a reference to a row that does not exist.
func isForeignKey(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23503"
}

// isUnique is true for a duplicate of a unique key.
func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
