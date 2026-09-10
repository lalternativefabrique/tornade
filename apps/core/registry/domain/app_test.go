package domain

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func sealed(s string) []byte { return []byte("sealed:" + s) }

func TestRegisterKeepsOnlyTheLastFourOfTheKey(t *testing.T) {
	a, err := Register("partage", sealed("1"), "abcdef", t0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Last4 != "cdef" || !a.Active() {
		t.Fatalf("app = %+v", a)
	}
}

func TestRegisterRefusesANameASignatureCouldNotCarry(t *testing.T) {
	for _, name := range []string{"", "Partage", "par tage", "par:tage", "-x"} {
		if _, err := Register(name, sealed("1"), "k", t0); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Register(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// A rotation must not break the app in the minutes before it redeploys, nor
// the URLs it already handed out: the old key keeps verifying for Grace.
func TestRotateKeepsThePreviousKeyForGrace(t *testing.T) {
	a, _ := Register("lalter", sealed("1"), "k1", t0)
	if err := a.Rotate(sealed("2"), "n1", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if string(a.Current) != "sealed:2" || string(a.Previous) != "sealed:1" {
		t.Fatalf("rotate kept the wrong key: %+v", a)
	}
	within := t0.Add(time.Hour + Grace/2)
	if !a.PreviousValidAt(within) || a.GraceUntil(within) == nil {
		t.Fatal("previous key refused inside the grace period")
	}
	after := t0.Add(time.Hour + Grace)
	if a.PreviousValidAt(after) || a.GraceUntil(after) != nil {
		t.Fatal("previous key still valid after the grace period")
	}
}

func TestRevokeEndsEveryKeyAtOnce(t *testing.T) {
	a, _ := Register("synthiz", sealed("1"), "k1", t0)
	a.Rotate(sealed("2"), "n1", t0)
	a.Revoke(t0.Add(time.Minute))
	if a.Active() || a.PreviousValidAt(t0.Add(time.Minute)) {
		t.Fatal("a revoked app still speaks")
	}
	if err := a.Rotate(sealed("3"), "x", t0.Add(time.Hour)); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Rotate after revoke = %v, want ErrRevoked", err)
	}
}
