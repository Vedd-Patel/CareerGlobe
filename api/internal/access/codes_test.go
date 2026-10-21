package access

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var pepper = bytes.Repeat([]byte("p"), 32)

func TestNewCodeShapeAndAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		c, err := NewCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(c) != 12 || !strings.HasPrefix(c, "CG-") || c[7] != '-' {
			t.Fatalf("bad shape %q", c)
		}
		if Normalize(c) != c {
			t.Fatalf("code %q does not normalize to itself", c)
		}
		if strings.ContainsAny(c[3:], "01OIL") {
			t.Fatalf("look-alike character in %q", c)
		}
		seen[c] = true
	}
	if len(seen) < 199 {
		t.Fatalf("codes repeat: %d unique of 200", len(seen))
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"CG-7K4P-2QXM":   "CG-7K4P-2QXM",
		"cg-7k4p-2qxm":   "CG-7K4P-2QXM",
		" 7K4P 2QXM ":    "CG-7K4P-2QXM",
		"cg7k4p2qxm":     "CG-7K4P-2QXM",
		"CG-7K4P-2QX0":   "", // 0 is not in the alphabet
		"CG-7K4P":        "",
		"CG-7K4P-2QXM-1": "",
		"":               "",
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHashIsPepperedAndStable(t *testing.T) {
	h1, _ := NewHasher(pepper)
	h2, _ := NewHasher(bytes.Repeat([]byte("q"), 32))
	a, b := h1.Hash("CG-7K4P-2QXM"), h1.Hash("CG-7K4P-2QXM")
	if !bytes.Equal(a, b) || len(a) != 32 {
		t.Fatal("hash not stable")
	}
	if bytes.Equal(a, h2.Hash("CG-7K4P-2QXM")) {
		t.Fatal("different peppers must give different hashes")
	}
}

func TestShortPepperRejected(t *testing.T) {
	if _, err := NewHasher([]byte("short")); err != ErrShortPepper {
		t.Fatalf("err = %v", err)
	}
}

func TestTicketRoundTripExpiryAndTampering(t *testing.T) {
	h, _ := NewHasher(pepper)
	now := time.Unix(1_800_000_000, 0)
	s, issued, err := h.IssueTicket(42, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := h.ParseTicket(s, now.Add(9*time.Minute))
	if err != nil || got != issued || got.CodeID != 42 {
		t.Fatalf("parse = %+v, %v", got, err)
	}
	if _, err := h.ParseTicket(s, now.Add(TicketTTL)); err != ErrTicketExpired {
		t.Fatalf("expired ticket: err = %v", err)
	}
	payload, sig, _ := strings.Cut(s, ".")
	for _, bad := range []string{"", "nodot", payload + ".AAAA", "e30." + sig, payload[:len(payload)-2] + "xx." + sig} {
		if _, err := h.ParseTicket(bad, now); err != ErrBadTicket {
			t.Errorf("ParseTicket(%q) err = %v, want ErrBadTicket", bad, err)
		}
	}
	other, _ := NewHasher(bytes.Repeat([]byte("q"), 32))
	if _, err := other.ParseTicket(s, now); err != ErrBadTicket {
		t.Fatalf("ticket from another pepper accepted: %v", err)
	}
}
