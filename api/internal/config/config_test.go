package config

import (
	"strings"
	"testing"
)

func env(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"API_DATABASE_URL", "DATABASE_URL", "FIREBASE_PROJECT_ID", "WEB_ORIGINS", "DEV_AUTH",
		"RATE_LIMIT_PER_MINUTE", "RESUME_ENCRYPTION_KEY"} {
		t.Setenv(k, "")
	}
	t.Setenv("DATABASE_URL", "postgres://localhost/x")
	t.Setenv("ACCESS_CODE_PEPPER", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=") // 32 zero bytes: a test value
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestDevAuthOnlyRunsLocally(t *testing.T) {
	env(t, map[string]string{"DEV_AUTH": "1", "WEB_ORIGINS": "http://localhost:5173,http://127.0.0.1:4173"})
	if c, err := Load(); err != nil || !c.DevAuth {
		t.Fatalf("local dev sign-in refused: DevAuth=%v err=%v", c.DevAuth, err)
	}

	env(t, map[string]string{"DEV_AUTH": "1", "WEB_ORIGINS": "http://localhost:5173,https://careerglobe.example"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "local development only") {
		t.Fatalf("DEV_AUTH with a public origin: err = %v, want a refusal", err)
	}

	env(t, map[string]string{"DEV_AUTH": "1", "FIREBASE_PROJECT_ID": "some-project"})
	if _, err := Load(); err == nil {
		t.Fatal("DEV_AUTH together with a Firebase project must be refused")
	}

	env(t, nil)
	if c, err := Load(); err != nil || c.DevAuth {
		t.Fatalf("DevAuth=%v err=%v without DEV_AUTH", c.DevAuth, err)
	}
}

func TestRateLimitSetting(t *testing.T) {
	env(t, nil)
	if c, _ := Load(); c.RatePerMinute != 600 {
		t.Errorf("default = %d, want 600", c.RatePerMinute)
	}
	env(t, map[string]string{"RATE_LIMIT_PER_MINUTE": "0"})
	if c, err := Load(); err != nil || c.RatePerMinute != 0 {
		t.Errorf("0 must turn the limit off: %d, %v", c.RatePerMinute, err)
	}
	env(t, map[string]string{"RATE_LIMIT_PER_MINUTE": "lots"})
	if _, err := Load(); err == nil {
		t.Error("a non-number must be refused")
	}
}
