package registry

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/lalternativefabrique/tornade/core/registry/domain"
)

// keyLister is what KeySource needs from the store: every app, whole.
type keyLister interface {
	List(ctx context.Context) ([]*domain.App, error)
}

// KeySource answers the speak guard's two questions: which secrets may an
// issuer's signature verify against, and which application presented this
// app key. It merges the registry with the pairs read from the environment,
// so a deployment that predates the registry keeps working and the registry
// wins where both name the same issuer.
type KeySource struct {
	apps       keyLister
	envSigning map[string]string
	envApp     map[string]string
	ttl        time.Duration
	now        func() time.Time

	mu        sync.Mutex
	signing   map[string][]string
	issuerOf  map[string]string
	fetchedAt time.Time
}

// NewKeySource reads the registry through apps, or only the environment when
// apps is nil.
func NewKeySource(apps keyLister, envSigning, envApp map[string]string) *KeySource {
	return &KeySource{apps: apps, envSigning: envSigning, envApp: envApp, ttl: 15 * time.Second, now: time.Now}
}

// SigningKeys are the secrets issuer's signatures may verify against now.
func (k *KeySource) SigningKeys(issuer string) []string {
	signing, _ := k.snapshot()
	return signing[issuer]
}

// IssuerOf names the application holding key, if any.
func (k *KeySource) IssuerOf(key string) (string, bool) {
	_, issuerOf := k.snapshot()
	name, ok := issuerOf[key]
	return name, ok
}

// Invalidate drops the cache, so a registration or rotation is honoured on
// the next request rather than after the refresh interval.
func (k *KeySource) Invalidate() {
	k.mu.Lock()
	k.fetchedAt = time.Time{}
	k.mu.Unlock()
}

func (k *KeySource) snapshot() (map[string][]string, map[string]string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := k.now()
	if k.signing != nil && now.Sub(k.fetchedAt) < k.ttl {
		return k.signing, k.issuerOf
	}
	signing := map[string][]string{}
	issuerOf := map[string]string{}
	for issuer, secret := range k.envSigning {
		signing[issuer] = []string{secret}
	}
	for issuer, secret := range k.envApp {
		issuerOf[secret] = issuer
	}
	if k.apps != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		apps, err := k.apps.List(ctx)
		cancel()
		if err != nil {
			log.Printf("registry: keys not refreshed: %v", err)
			if k.signing != nil {
				return k.signing, k.issuerOf
			}
		}
		for _, a := range apps {
			if keys := a.SigningKeys(now); len(keys) > 0 {
				signing[a.Name] = keys
			}
			for _, key := range a.AppKeys(now) {
				issuerOf[key] = a.Name
			}
		}
	}
	k.signing, k.issuerOf, k.fetchedAt = signing, issuerOf, now
	return signing, issuerOf
}
