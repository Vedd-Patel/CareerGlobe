package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/nightly"
)

// rematch rebuilds one member's matches in the background after they change
// their profile or preferences, so the Matches screen is not empty until the
// nightly run. Saves that arrive close together queue up per member, so the
// last one always runs last on the newest data.
func (s *Server) rematch(userID int64) {
	if s.cfg.Pool == nil {
		return
	}
	lock, _ := s.rematching.LoadOrStore(userID, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	go func() {
		mu.Lock()
		defer mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if n, err := nightly.MatchOne(ctx, s.cfg.Pool, userID, time.Now()); err != nil {
			slog.Error("rematch failed", "user", userID, "err", err)
		} else {
			slog.Info("rematched", "user", userID, "kept", n)
		}
	}()
}
