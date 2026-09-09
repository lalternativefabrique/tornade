package config

import "testing"

// Both key variables take the same "issuer:secret" shape. Two formats that
// look alike and are not is how a secret gets pasted into the wrong one and
// silently authenticates nobody.
func TestAppKeysAreReadByIssuer(t *testing.T) {
	t.Setenv("SPEAK_APP_KEYS", "lalter:aaa, synthiz:bbb")

	keys := Load().AppKeys
	if got := keys["lalter"]; len(got) != 1 || got[0] != "aaa" {
		t.Errorf("keys[lalter] = %q, want aaa", got)
	}
	if got := keys["synthiz"]; len(got) != 1 || got[0] != "bbb" {
		t.Errorf("keys[synthiz] = %q, want bbb", got)
	}
	if len(keys) != 2 {
		t.Errorf("got %d keys, want 2", len(keys))
	}
}

// An entry with no issuer is dropped rather than read as a bare secret: it
// would authenticate a caller nobody can name, and the operator who wrote it
// believes the whole line took effect.
func TestMalformedKeysAreDropped(t *testing.T) {
	t.Setenv("SPEAK_APP_KEYS", "no-issuer,lalter:aaa,:empty,synthiz:")

	keys := Load().AppKeys
	if len(keys) != 1 || len(keys["lalter"]) != 1 || keys["lalter"][0] != "aaa" {
		t.Errorf("keys = %v, want only the well-formed pair", keys)
	}
}

func TestNoKeysConfiguredIsEmpty(t *testing.T) {
	t.Setenv("SPEAK_APP_KEYS", "")
	t.Setenv("SPEAK_SIGNING_KEYS", "")

	cfg := Load()
	if len(cfg.AppKeys) != 0 || len(cfg.SigningKeys) != 0 {
		t.Errorf("keys = %v / %v, want both empty", cfg.AppKeys, cfg.SigningKeys)
	}
}

func TestFetchProxyIsReadFromTheEnvironment(t *testing.T) {
	t.Setenv("FETCH_PROXY", "http://user:pass@gate.decodo.com:7000")
	if got := Load().FetchProxy; got != "http://user:pass@gate.decodo.com:7000" {
		t.Errorf("FetchProxy = %q", got)
	}
}

func TestFetchProxyDefaultsToDirect(t *testing.T) {
	t.Setenv("FETCH_PROXY", "")
	if got := Load().FetchProxy; got != "" {
		t.Errorf("FetchProxy = %q, want empty so the fetch goes direct", got)
	}
}

// An issuer listed twice holds both secrets: a rotation by hand, the old key
// valid while the application redeploys with the new one.
func TestAnIssuerMayHoldSeveralSecrets(t *testing.T) {
	t.Setenv("SPEAK_SIGNING_KEYS", "lalter:new,lalter:old")

	keys := Load().SigningKeys
	if got := keys["lalter"]; len(got) != 2 || got[0] != "new" || got[1] != "old" {
		t.Errorf("keys[lalter] = %q, want both, in order", got)
	}
}
