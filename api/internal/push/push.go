// Package push sends notifications to phones and browsers with Firebase Cloud Messaging.
package push

import (
	"context"
	"errors"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
)

// Message is one notification to one device.
type Message struct {
	Token string
	Title string
	Body  string
	Link  string // path inside the app, e.g. /matches
}

// ErrUnregistered: the device token is no longer valid; delete it.
var ErrUnregistered = errors.New("push: token unregistered")

// Sender delivers messages.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// FCM sends through Firebase Cloud Messaging (credentials from GOOGLE_APPLICATION_CREDENTIALS).
type FCM struct {
	client *messaging.Client
}

// NewFCM connects to Firebase.
func NewFCM(ctx context.Context, projectID string) (*FCM, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	c, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	return &FCM{client: c}, nil
}

// Send implements Sender.
func (f *FCM) Send(ctx context.Context, m Message) error {
	_, err := f.client.Send(ctx, &messaging.Message{
		Token:        m.Token,
		Notification: &messaging.Notification{Title: m.Title, Body: m.Body},
		Data:         map[string]string{"link": m.Link},
		Android:      &messaging.AndroidConfig{Priority: "normal"},
		Webpush:      &messaging.WebpushConfig{FCMOptions: &messaging.WebpushFCMOptions{Link: m.Link}},
	})
	if messaging.IsUnregistered(err) || messaging.IsInvalidArgument(err) {
		return ErrUnregistered
	}
	return err
}

// Recorder keeps messages instead of sending them (tests, or no Firebase yet).
type Recorder struct {
	mu   sync.Mutex
	Sent []Message
	Dead map[string]bool // tokens to report as unregistered
}

// Send implements Sender.
func (r *Recorder) Send(_ context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Dead[m.Token] {
		return ErrUnregistered
	}
	r.Sent = append(r.Sent, m)
	return nil
}
