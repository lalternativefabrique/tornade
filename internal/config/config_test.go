package config

import "testing"

// Both key variables take the same "issuer:secret" shape. Two formats that
// look alike and are not is how a secret gets pasted into the wrong one and
// silently authenticates nobody.
func TestAppKeysAreReadByIssuerAndLookedUpBySecret(t *testing.T) {
	t.Setenv("SPEAK_APP_KEYS", "lalter:aaa, synthiz:bbb")

	keys := Load().AppKeys
	if got := keys["aaa"]; got != "lalter" {
		t.Errorf("keys[aaa] = %q, want lalter", got)
	}
	if got := keys["bbb"]; got != "synthiz" {
		t.Errorf("keys[bbb] = %q, want synthiz", got)
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
	if len(keys) != 1 || keys["aaa"] != "lalter" {
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
