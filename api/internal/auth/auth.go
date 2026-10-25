// Package auth verifies Firebase ID tokens.
package auth

import (
	"context"
	"errors"
	"strings"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// Identity is what a verified token says about the caller.
type Identity struct {
	UID           string
	Email         string
	Name          string
	EmailVerified bool
	Provider      string // "password", "google.com", "apple.com", ...
}

// ErrInvalidToken means the token is missing, malformed, expired or not ours.
var ErrInvalidToken = errors.New("auth: invalid token")

// Verifier checks a Firebase ID token.
type Verifier interface {
	Verify(ctx context.Context, idToken string) (Identity, error)
}

// Firebase verifies tokens with the Firebase Admin SDK (Google's public keys,
// cached by the SDK). Credentials come from GOOGLE_APPLICATION_CREDENTIALS.
type Firebase struct {
	client *fbauth.Client
}

// NewFirebase connects to the Firebase project.
func NewFirebase(ctx context.Context, projectID string) (*Firebase, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return &Firebase{client: client}, nil
}

// Verify implements Verifier. Revoked sessions (CheckRevoked) are rejected too.
func (f *Firebase) Verify(ctx context.Context, idToken string) (Identity, error) {
	if idToken == "" {
		return Identity{}, ErrInvalidToken
	}
	tok, err := f.client.VerifyIDTokenAndCheckRevoked(ctx, idToken)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	id := Identity{UID: tok.UID, Provider: tok.Firebase.SignInProvider}
	if v, ok := tok.Claims["email"].(string); ok {
		id.Email = v
	}
	if v, ok := tok.Claims["name"].(string); ok {
		id.Name = v
	}
	if v, ok := tok.Claims["email_verified"].(bool); ok {
		id.EmailVerified = v
	}
	return id, nil
}

// Static is a Verifier for tests and local development: token -> identity.
type Static map[string]Identity

// Verify implements Verifier.
func (s Static) Verify(_ context.Context, idToken string) (Identity, error) {
	if id, ok := s[idToken]; ok {
		return id, nil
	}
	return Identity{}, ErrInvalidToken
}

// Dev accepts "dev:<uid>:<email>" tokens. For local development only: main
// enables it when DEV_AUTH=1 and no Firebase project is configured, and the
// API only ever listens on localhost.
type Dev struct{}

// Verify implements Verifier.
func (Dev) Verify(_ context.Context, idToken string) (Identity, error) {
	parts := strings.SplitN(idToken, ":", 3)
	if len(parts) != 3 || parts[0] != "dev" || parts[1] == "" {
		return Identity{}, ErrInvalidToken
	}
	return Identity{UID: "dev-" + parts[1], Email: parts[2], Name: parts[1], EmailVerified: true, Provider: "dev"}, nil
}
