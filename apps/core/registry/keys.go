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

// opener unseals a stored key.
type opener interface {
	Decrypt(blob []byte) (string, error)
}

// KeySource answers the speak guard's two questions: which keys may an
// issuer's signature verify against, and which application presented this
// key. It merges the registry with the pairs read from the environment, so a
// deployment that predates the registry keeps working and the registry wins
// where both name the same issuer.
type KeySource struct {
	apps   keyLister
	cipher opener
	env    map[string][]string
	ttl    time.Duration
	now    func() time.Time

	mu        sync.Mutex
	keys      map[string][]string
	issuerOf  map[string]string
	fetchedAt time.Time
}

// NewKeySource reads the registry through apps, or only the environment when
// apps is nil.
func NewKeySource(apps keyLister, cipher opener, env map[string][]string) *KeySource {
	return &KeySource{apps: apps, cipher: cipher, env: env, ttl: 15 * time.Second, now: time.Now}
}

// Keys are the keys issuer's signatures may verify against now.
func (k *KeySource) Keys(issuer string) []string {
	keys, _ := k.snapshot()
	return keys[issuer]
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
	if k.keys != nil && now.Sub(k.fetchedAt) < k.ttl {
		return k.keys, k.issuerOf
	}
	keys := map[string][]string{}
	issuerOf := map[string]string{}
	if k.apps != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		apps, err := k.apps.List(ctx)
		cancel()
		if err != nil {
			log.Printf("registry: keys not refreshed: %v", err)
			if k.keys != nil {
				return k.keys, k.issuerOf
			}
		}
		for _, a := range apps {
			if !a.Active() {
				continue
			}
			sealed := [][]byte{a.Current}
			if a.PreviousValidAt(now) {
				sealed = append(sealed, a.Previous)
			}
			var secrets []string
			for _, blob := range sealed {
				s, err := k.cipher.Decrypt(blob)
				if err != nil {
					log.Printf("registry: %s: %v", a.Name, err)
					continue
				}
				secrets = append(secrets, s)
				issuerOf[s] = a.Name
			}
			if len(secrets) > 0 {
				keys[a.Name] = secrets
			}
		}
	}
	for issuer, secrets := range k.env {
		if _, registered := keys[issuer]; registered {
			continue
		}
		keys[issuer] = append([]string(nil), secrets...)
		for _, secret := range secrets {
			issuerOf[secret] = issuer
		}
	}
	k.keys, k.issuerOf, k.fetchedAt = keys, issuerOf, now
	return keys, issuerOf
}
