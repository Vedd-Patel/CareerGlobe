package resume

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vedd-Patel/CareerGlobe/api/internal/db"
)

// ConsentText is the exact checkbox text the apps show (spec: Privacy).
const ConsentText = "I agree to share my resume and profile details with CareerGlobe so it can find more " +
	"relevant, better-matched jobs for me. I have read the Privacy Policy."

// ErrNoConsent: the upload came without the ticked consent box.
var ErrNoConsent = errors.New("resume: consent is required")

// Cities the parser looks for in the contact block (and their old names).
var Cities = []string{
	"Bengaluru", "Bangalore", "Mumbai", "Bombay", "Navi Mumbai", "Thane", "Delhi", "New Delhi", "Gurugram",
	"Gurgaon", "Noida", "Greater Noida", "Faridabad", "Ghaziabad", "Hyderabad", "Secunderabad", "Chennai", "Pune",
	"Kolkata", "Ahmedabad", "Gandhinagar", "Jaipur", "Kochi", "Cochin", "Thiruvananthapuram", "Trivandrum",
	"Coimbatore", "Indore", "Bhopal", "Chandigarh", "Mohali", "Lucknow", "Bhubaneswar", "Nagpur", "Vadodara",
	"Surat", "Mysuru", "Mysore", "Mangaluru", "Mangalore", "Visakhapatnam", "Vizag", "Goa", "Patna", "Ranchi",
	"Guwahati", "Dehradun", "Vijayawada", "Madurai", "Nashik", "Aurangabad", "Raipur", "Jabalpur", "Kanpur",
}

// Service handles uploads: extract, parse, encrypt, record consent.
type Service struct {
	pool  *pgxpool.Pool
	q     *db.Queries
	vault *Vault
	now   func() time.Time
}

// NewService wires uploads to the database and the encrypted file store.
func NewService(pool *pgxpool.Pool, vault *Vault) *Service {
	return &Service{pool: pool, q: db.New(pool), vault: vault, now: time.Now}
}

// Consent is the proof stored with each upload (DPDP).
type Consent struct {
	Given         bool
	PolicyVersion string
	IP            *netip.Addr
	UserAgent     string
}

// Upload stores a new resume (replacing the old one) and returns the parsed
// draft for the user to confirm (S09). The profile's draft fields are replaced
// but its confirmed state is kept until the user confirms again.
func (s *Service) Upload(ctx context.Context, userID int64, name string, data []byte, c Consent) (Draft, error) {
	if !c.Given {
		return Draft{}, ErrNoConsent
	}
	text, err := Text(data)
	if err != nil {
		return Draft{}, err
	}
	draft := Parse(text, Cities, s.now())
	kind := Kind(data)
	file, err := s.vault.Put(userID, data)
	if err != nil {
		return Draft{}, err
	}
	var old string
	parsed := s.now()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if prev, err := q.GetResume(ctx, userID); err == nil {
			old = prev.FilePath
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if len(name) > 200 {
			name = name[:200]
		}
		if _, err := q.UpsertResume(ctx, db.UpsertResumeParams{
			UserID: userID, FilePath: file, OriginalName: &name, ContentType: ContentType(kind),
			SizeBytes: int32(len(data)), ParsedAt: &parsed,
		}); err != nil {
			return err
		}
		ua := c.UserAgent
		if len(ua) > 300 {
			ua = ua[:300]
		}
		if err := q.RecordConsent(ctx, db.RecordConsentParams{
			UserID: userID, PolicyVersion: c.PolicyVersion, Ip: c.IP, UserAgent: &ua,
		}); err != nil {
			return err
		}
		_, err := q.UpsertProfile(ctx, db.UpsertProfileParams{
			UserID: userID, Skills: draft.Skills, Titles: draft.Titles, YearsExp: draft.YearsExp,
			CurrentCity: draft.CurrentCity, Education: draft.Education, Confirm: false,
		})
		return err
	})
	if err != nil {
		_ = s.vault.Delete(file)
		return Draft{}, err
	}
	if old != "" && old != file {
		if err := s.vault.Delete(old); err != nil {
			slog.Warn("resume: old file not deleted", "err", err)
		}
	}
	return draft, nil
}

// DeleteFiles removes a user's stored resume file (account deletion).
func (s *Service) DeleteFiles(ctx context.Context, userID int64) error {
	r, err := s.q.GetResume(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.vault.Delete(r.FilePath)
}
