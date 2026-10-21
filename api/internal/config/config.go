// Package config reads the API's settings from the environment (.env on the server).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config holds every setting the API and the nightly jobs need. Secrets are
// kept as bytes and never printed.
type Config struct {
	DatabaseURL      string
	Port             string
	FirebaseProject  string
	AdminUIDs        map[string]bool
	Pepper           []byte // ACCESS_CODE_PEPPER, base64
	ResumeKey        []byte // RESUME_ENCRYPTION_KEY, base64, 32 bytes
	ResumeDir        string
	OpsToken         string // bearer token for the GitHub watchdog (/v1/ops)
	WebOrigins       []string
	PolicyVersion    string
	HealthMatchURL   string
	TrustProxyHeader string // header with the client IP set by Cloudflare
}

// Load reads the environment. Required: a database URL and the access-code
// pepper (every request depends on them).
func Load() (Config, error) {
	c := Config{
		DatabaseURL:      first(os.Getenv("API_DATABASE_URL"), os.Getenv("DATABASE_URL")),
		Port:             first(os.Getenv("API_PORT"), "8080"),
		FirebaseProject:  os.Getenv("FIREBASE_PROJECT_ID"),
		AdminUIDs:        set(os.Getenv("ADMIN_FIREBASE_UIDS")),
		ResumeDir:        first(os.Getenv("RESUME_STORAGE_DIR"), "/var/lib/careerglobe/resumes"),
		OpsToken:         os.Getenv("OPS_TOKEN"),
		WebOrigins:       list(os.Getenv("WEB_ORIGINS")),
		PolicyVersion:    first(os.Getenv("PRIVACY_POLICY_VERSION"), "2026-10-01"),
		HealthMatchURL:   os.Getenv("HEALTHCHECK_MATCH_URL"),
		TrustProxyHeader: first(os.Getenv("CLIENT_IP_HEADER"), "CF-Connecting-IP"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("API_DATABASE_URL or DATABASE_URL must be set")
	}
	var err error
	if c.Pepper, err = decode("ACCESS_CODE_PEPPER", 32); err != nil {
		return c, err
	}
	if os.Getenv("RESUME_ENCRYPTION_KEY") != "" {
		if c.ResumeKey, err = decode("RESUME_ENCRYPTION_KEY", 32); err != nil {
			return c, err
		}
		if len(c.ResumeKey) != 32 {
			return c, errors.New("RESUME_ENCRYPTION_KEY must decode to exactly 32 bytes")
		}
	}
	return c, nil
}

func decode(name string, minLen int) ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, fmt.Errorf("%s must be set (base64, %d+ random bytes: openssl rand -base64 32)", name, minLen)
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64", name)
	}
	if len(b) < minLen {
		return nil, fmt.Errorf("%s must decode to at least %d bytes", name, minLen)
	}
	return b, nil
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func list(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, v := range list(s) {
		m[v] = true
	}
	return m
}
