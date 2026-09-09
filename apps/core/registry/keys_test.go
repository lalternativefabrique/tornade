package registry

import (
	"context"
	"testing"
	"time"

	"github.com/lalternativefabrique/tornade/core/registry/domain"
)

type fakeLister struct {
	apps  []*domain.App
	calls int
}

func (f *fakeLister) List(context.Context) ([]*domain.App, error) {
	f.calls++
	return f.apps, nil
}

func TestRegistryWinsOverTheEnvironmentAndBothAreServed(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	partage, _ := domain.Register("partage", now)
	lister := &fakeLister{apps: []*domain.App{partage}}
	k := NewKeySource(lister,
		map[string]string{"partage": "old-env-secret", "lalter": "lalter-secret"},
		map[string]string{"lalter": "lalter-app-key"})
	k.now = func() time.Time { return now }

	if got := k.SigningKeys("partage"); len(got) != 1 || got[0] != partage.SigningKey {
		t.Fatalf("partage keys = %v, want the registry's", got)
	}
	if got := k.SigningKeys("lalter"); len(got) != 1 || got[0] != "lalter-secret" {
		t.Fatalf("lalter keys = %v, want the environment's", got)
	}
	if name, ok := k.IssuerOf(partage.AppKey); !ok || name != "partage" {
		t.Fatalf("IssuerOf(registry key) = %q,%v", name, ok)
	}
	if name, ok := k.IssuerOf("lalter-app-key"); !ok || name != "lalter" {
		t.Fatalf("IssuerOf(env key) = %q,%v", name, ok)
	}
	if _, ok := k.IssuerOf("nope"); ok {
		t.Fatal("an unknown key found an issuer")
	}
}

func TestKeysAreCachedUntilInvalidatedOrStale(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	lister := &fakeLister{}
	k := NewKeySource(lister, nil, nil)
	k.now = func() time.Time { return now }

	k.SigningKeys("x")
	k.SigningKeys("x")
	if lister.calls != 1 {
		t.Fatalf("calls = %d, want one fetch within the ttl", lister.calls)
	}
	k.Invalidate()
	k.SigningKeys("x")
	if lister.calls != 2 {
		t.Fatalf("calls = %d, want a refetch after Invalidate", lister.calls)
	}
	now = now.Add(time.Minute)
	k.SigningKeys("x")
	if lister.calls != 3 {
		t.Fatalf("calls = %d, want a refetch once stale", lister.calls)
	}
}
