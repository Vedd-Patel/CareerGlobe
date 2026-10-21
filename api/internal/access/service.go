package access

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
)

const (
	MaxFailedAttempts = 5
	AttemptWindow     = 15 * time.Minute
	AllowedCacheTTL   = 60 * time.Second
)

// Reasons a code or account is refused; the API turns them into error codes.
var (
	ErrInvalidCode     = errors.New("invalid_code")
	ErrCodeRevoked     = errors.New("code_revoked")
	ErrCodeExpired     = errors.New("code_expired")
	ErrCodeUsedUp      = errors.New("code_used_up")
	ErrTooManyAttempts = errors.New("too_many_attempts")
	ErrTicketUsed      = errors.New("ticket_used")
	ErrNoAccess        = errors.New("access_code_required")
	ErrAccessRemoved   = errors.New("access_removed")
)

// Account is a signed-in user who is allowed in.
type Account struct {
	UserID int64
	UID    string
}

// Service runs the code checks against the database.
type Service struct {
	pool   *pgxpool.Pool
	q      *db.Queries
	hasher *Hasher
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	acc     Account
	err     error
	expires time.Time
}

// NewService wires the code checks to the database.
func NewService(pool *pgxpool.Pool, hasher *Hasher) *Service {
	return &Service{pool: pool, q: db.New(pool), hasher: hasher, now: time.Now, cache: map[string]cached{}}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Verify checks a typed code and returns a single-use ticket. It does not use
// the code up. Failed tries are counted per IP and per device.
func (s *Service) Verify(ctx context.Context, input string, ip *netip.Addr, deviceID string) (string, time.Time, error) {
	var dev *string
	if deviceID != "" {
		dev = &deviceID
	}
	failed, err := s.q.CountFailedAttempts(ctx, db.CountFailedAttemptsParams{
		Since: s.now().Add(-AttemptWindow), Ip: ip, DeviceID: dev,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if failed >= MaxFailedAttempts {
		return "", time.Time{}, ErrTooManyAttempts
	}

	code, reason := s.lookup(ctx, input)
	if err := s.q.RecordAttempt(ctx, db.RecordAttemptParams{Ip: ip, DeviceID: dev, Success: reason == nil}); err != nil {
		return "", time.Time{}, err
	}
	if reason != nil {
		return "", time.Time{}, reason
	}
	ticket, t, err := s.hasher.IssueTicket(code.ID, s.now())
	if err != nil {
		return "", time.Time{}, err
	}
	return ticket, time.Unix(t.Expires, 0), nil
}

func (s *Service) lookup(ctx context.Context, input string) (db.AccessCode, error) {
	norm := Normalize(input)
	if norm == "" {
		return db.AccessCode{}, ErrInvalidCode
	}
	code, err := s.q.GetCodeByHash(ctx, s.hasher.Hash(norm))
	if errors.Is(err, pgx.ErrNoRows) {
		return code, ErrInvalidCode
	}
	if err != nil {
		return code, err
	}
	return code, usable(code, s.now())
}

func usable(c db.AccessCode, now time.Time) error {
	switch {
	case c.Revoked:
		return ErrCodeRevoked
	case c.ExpiresAt != nil && !now.Before(*c.ExpiresAt):
		return ErrCodeExpired
	case c.UsedCount >= c.MaxUses:
		return ErrCodeUsedUp
	}
	return nil
}

// Redeem links a Firebase account to the code behind a ticket, in one
// transaction with the code row locked, so two people racing for the last use
// cannot both get in. Redeeming again for an already linked account is a no-op.
func (s *Service) Redeem(ctx context.Context, ticket, uid, name, email string) (db.User, error) {
	t, err := s.hasher.ParseTicket(ticket, s.now())
	if err != nil {
		return db.User{}, err
	}
	var user db.User
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if existing, err := q.GetAccountByUID(ctx, uid); err == nil {
			user = db.User{ID: existing.ID, FirebaseUid: existing.FirebaseUid, Name: existing.Name,
				Email: existing.Email, CreatedAt: existing.CreatedAt, LastLoginAt: existing.LastLoginAt}
			return nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		n, err := q.UseTicket(ctx, db.UseTicketParams{TicketID: t.ID, ExpiresAt: time.Unix(t.Expires, 0)})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrTicketUsed
		}
		code, err := q.LockCode(ctx, t.CodeID)
		if err != nil {
			return err
		}
		if err := usable(code, s.now()); err != nil {
			return err
		}
		if err := q.UseCode(ctx, code.ID); err != nil {
			return err
		}
		user, err = q.CreateUser(ctx, db.CreateUserParams{
			FirebaseUid: uid, AccessCodeID: code.ID, Name: nonEmpty(name), Email: nonEmpty(email),
		})
		return err
	})
	if err == nil {
		s.Forget(uid)
	}
	return user, err
}

// Allowed is the per-request check: the Firebase account is linked to a code
// that is not revoked. Answers are cached for a minute, so revoking a code
// locks its users out within a minute.
func (s *Service) Allowed(ctx context.Context, uid string) (Account, error) {
	now := s.now()
	s.mu.Lock()
	if c, ok := s.cache[uid]; ok && now.Before(c.expires) {
		s.mu.Unlock()
		return c.acc, c.err
	}
	s.mu.Unlock()

	var acc Account
	row, err := s.q.GetAccountByUID(ctx, uid)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = ErrNoAccess
	case err != nil:
		return Account{}, err // database trouble: not cached
	case row.CodeRevoked:
		err = ErrAccessRemoved
	default:
		acc = Account{UserID: row.ID, UID: uid}
		_ = s.q.TouchLogin(ctx, row.ID)
	}
	s.mu.Lock()
	if len(s.cache) > 50_000 {
		s.cache = map[string]cached{}
	}
	s.cache[uid] = cached{acc: acc, err: err, expires: now.Add(AllowedCacheTTL)}
	s.mu.Unlock()
	return acc, err
}

// Forget drops a cached answer (after redeem or account deletion).
func (s *Service) Forget(uid string) {
	s.mu.Lock()
	delete(s.cache, uid)
	s.mu.Unlock()
}

// CreateCode makes a new code and returns it in plain text. This is the only
// time the plain code exists; the database keeps its hash.
func (s *Service) CreateCode(ctx context.Context, label string, maxUses int32, expires *time.Time) (string, db.AccessCode, error) {
	if maxUses <= 0 {
		maxUses = 1
	}
	for range 5 {
		plain, err := NewCode()
		if err != nil {
			return "", db.AccessCode{}, err
		}
		row, err := s.q.CreateCode(ctx, db.CreateCodeParams{
			CodeHash: s.hasher.Hash(plain), Label: label, MaxUses: maxUses, ExpiresAt: expires,
		})
		if err == nil {
			return plain, row, nil
		}
		if !isUniqueViolation(err) {
			return "", db.AccessCode{}, err
		}
	}
	return "", db.AccessCode{}, errors.New("access: could not generate a unique code")
}

// SetRevoked revokes (or restores) a code; cached answers expire within a minute.
func (s *Service) SetRevoked(ctx context.Context, id int64, revoked bool) (db.AccessCode, error) {
	return s.q.SetCodeRevoked(ctx, db.SetCodeRevokedParams{ID: id, Revoked: revoked})
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
