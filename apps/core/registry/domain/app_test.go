package domain

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func TestRegisterMintsTwoDistinctKeys(t *testing.T) {
	a, err := Register("partage", t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.SigningKey) != 64 || len(a.AppKey) != 64 || a.SigningKey == a.AppKey {
		t.Fatalf("keys = %q / %q, want two distinct 64-hex secrets", a.SigningKey, a.AppKey)
	}
	if got := a.SigningKeys(t0); len(got) != 1 || got[0] != a.SigningKey {
		t.Fatalf("SigningKeys = %v", got)
	}
}

func TestRegisterRefusesANameASignatureCouldNotCarry(t *testing.T) {
	for _, name := range []string{"", "Partage", "par tage", "par:tage", "-x"} {
		if _, err := Register(name, t0); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Register(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

// A rotation must not break the app in the minutes before it redeploys, nor
// the URLs it already handed out: the old pair keeps verifying for Grace.
func TestRotateKeepsThePreviousPairForGrace(t *testing.T) {
	a, _ := Register("lalter", t0)
	oldSign, oldApp := a.SigningKey, a.AppKey

	if err := a.Rotate(t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if a.SigningKey == oldSign || a.AppKey == oldApp {
		t.Fatal("Rotate kept a key")
	}
	within := t0.Add(time.Hour + Grace/2)
	if got := a.SigningKeys(within); len(got) != 2 || got[0] != a.SigningKey || got[1] != oldSign {
		t.Fatalf("SigningKeys within grace = %v", got)
	}
	if got := a.AppKeys(within); len(got) != 2 || got[1] != oldApp {
		t.Fatalf("AppKeys within grace = %v", got)
	}
	after := t0.Add(time.Hour + Grace)
	if got := a.SigningKeys(after); len(got) != 1 || got[0] != a.SigningKey {
		t.Fatalf("SigningKeys after grace = %v, want the current one alone", got)
	}
}

func TestRevokeEndsEveryKeyAtOnce(t *testing.T) {
	a, _ := Register("synthiz", t0)
	a.Rotate(t0)
	a.Revoke(t0.Add(time.Minute))
	if a.Active() || len(a.SigningKeys(t0.Add(time.Minute))) != 0 || len(a.AppKeys(t0.Add(time.Minute))) != 0 {
		t.Fatal("a revoked app still has keys")
	}
	if err := a.Rotate(t0.Add(time.Hour)); !errors.Is(err, ErrRevoked) {
		t.Fatalf("Rotate after revoke = %v, want ErrRevoked", err)
	}
}
