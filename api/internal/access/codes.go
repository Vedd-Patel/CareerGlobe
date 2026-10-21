// Package access implements invite codes: generation, peppered hashing,
// verify tickets, and the redeem / allowed checks (spec: Private access).
package access

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Alphabet has no look-alike characters (no 0/O, 1/I/L).
const Alphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

const (
	codeGroups    = 2
	codeGroupLen  = 4
	codePrefix    = "CG"
	TicketTTL     = 10 * time.Minute
	minPepperSize = 32
)

var (
	ErrBadTicket     = errors.New("access: invalid ticket")
	ErrTicketExpired = errors.New("access: ticket expired")
	ErrShortPepper   = errors.New("access: pepper must be at least 32 bytes")
)

// Hasher turns codes into stored hashes and signs tickets, using a secret
// server pepper. Only HMAC-SHA256(pepper, code) is ever stored.
type Hasher struct {
	codeKey   []byte
	ticketKey []byte
}

// NewHasher derives separate keys for code hashing and ticket signing from
// the pepper (base64 in ACCESS_CODE_PEPPER, decoded by the caller).
func NewHasher(pepper []byte) (*Hasher, error) {
	if len(pepper) < minPepperSize {
		return nil, ErrShortPepper
	}
	return &Hasher{codeKey: derive(pepper, "code"), ticketKey: derive(pepper, "ticket")}, nil
}

func derive(pepper []byte, purpose string) []byte {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte("careerglobe/" + purpose))
	return m.Sum(nil)
}

// NewCode returns a random code like CG-7K4P-2QXM.
func NewCode() (string, error) {
	buf := make([]byte, codeGroups*codeGroupLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(codePrefix)
	for i, v := range buf {
		if i%codeGroupLen == 0 {
			b.WriteByte('-')
		}
		// 256 is not a multiple of 31; the tiny bias is irrelevant for 8 characters.
		b.WriteByte(Alphabet[int(v)%len(Alphabet)])
	}
	return b.String(), nil
}

// Normalize turns user input into the canonical code form, or "" when it
// cannot be a code. Case, spaces and dashes do not matter; the CG prefix is optional.
func Normalize(input string) string {
	s := strings.ToUpper(input)
	s = strings.NewReplacer(" ", "", "-", "", "\t", "").Replace(s)
	s = strings.TrimPrefix(s, codePrefix)
	if len(s) != codeGroups*codeGroupLen {
		return ""
	}
	for _, c := range s {
		if !strings.ContainsRune(Alphabet, c) {
			return ""
		}
	}
	return codePrefix + "-" + s[:codeGroupLen] + "-" + s[codeGroupLen:]
}

// Hash is the value stored in access_codes.code_hash.
func (h *Hasher) Hash(code string) []byte {
	m := hmac.New(sha256.New, h.codeKey)
	m.Write([]byte(code))
	return m.Sum(nil)
}

// Ticket is what /v1/access/verify hands out: proof that a valid code was
// entered, good for one redeem within TicketTTL.
type Ticket struct {
	CodeID  int64  `json:"c"`
	ID      string `json:"j"`
	Expires int64  `json:"e"`
}

// IssueTicket signs a ticket for a verified code.
func (h *Hasher) IssueTicket(codeID int64, now time.Time) (string, Ticket, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", Ticket{}, err
	}
	t := Ticket{CodeID: codeID, ID: base64.RawURLEncoding.EncodeToString(id), Expires: now.Add(TicketTTL).Unix()}
	payload, err := json.Marshal(t)
	if err != nil {
		return "", Ticket{}, err
	}
	p := base64.RawURLEncoding.EncodeToString(payload)
	return p + "." + base64.RawURLEncoding.EncodeToString(h.sign(p)), t, nil
}

// ParseTicket checks the signature and expiry. Single use is enforced by the
// redeem transaction (access_tickets_used), not here.
func (h *Hasher) ParseTicket(s string, now time.Time) (Ticket, error) {
	p, sig, ok := strings.Cut(s, ".")
	if !ok {
		return Ticket{}, ErrBadTicket
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, h.sign(p)) {
		return Ticket{}, ErrBadTicket
	}
	raw, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return Ticket{}, ErrBadTicket
	}
	var t Ticket
	if err := json.Unmarshal(raw, &t); err != nil || t.CodeID <= 0 || t.ID == "" {
		return Ticket{}, ErrBadTicket
	}
	if now.Unix() >= t.Expires {
		return Ticket{}, ErrTicketExpired
	}
	return t, nil
}

func (h *Hasher) sign(payload string) []byte {
	m := hmac.New(sha256.New, h.ticketKey)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
