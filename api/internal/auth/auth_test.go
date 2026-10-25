package auth

import (
	"context"
	"testing"
)

func TestDev(t *testing.T) {
	id, err := Dev{}.Verify(context.Background(), "dev:priya:priya@example.com")
	if err != nil || id.UID != "dev-priya" || id.Email != "priya@example.com" || !id.EmailVerified {
		t.Fatalf("id = %+v, err = %v", id, err)
	}
	for _, bad := range []string{"", "priya", "dev::x", "eyJhbGciOi.real.firebase"} {
		if _, err := (Dev{}).Verify(context.Background(), bad); err != ErrInvalidToken {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestStatic(t *testing.T) {
	s := Static{"t": {UID: "u"}}
	if id, err := s.Verify(context.Background(), "t"); err != nil || id.UID != "u" {
		t.Fatal(id, err)
	}
	if _, err := s.Verify(context.Background(), "x"); err != ErrInvalidToken {
		t.Fatal(err)
	}
}
