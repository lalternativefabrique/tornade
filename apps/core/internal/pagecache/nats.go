// Package pagecache keeps fetched pages in a NATS JetStream KV bucket, so
// every replica of vvaves reads the page one of them already fetched.
package pagecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/lalternative/packages/go/search/fetch"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	bucketName = "vvaves_pages"

	// A cache must never make a fetch slower than the fetch itself: a KV
	// lookup that is not back in this time is a miss, and a put that is not
	// done is dropped.
	getTimeout = 500 * time.Millisecond
	setTimeout = 2 * time.Second
)

type natsCache struct {
	kv jetstream.KeyValue
}

// NewNATS opens the bucket on nc, creating it with ttl and maxBytes when it
// does not exist yet. JetStream applies the TTL itself, so a stale page is
// gone for every replica at once rather than on the next lookup of one.
func NewNATS(nc *nats.Conn, ttl time.Duration, maxBytes int64) (fetch.Cache, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), setTimeout)
	defer cancel()

	kv, err := js.KeyValue(ctx, bucketName)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:   bucketName,
			TTL:      ttl,
			MaxBytes: maxBytes,
			Storage:  jetstream.FileStorage,
		})
	}
	if err != nil {
		return nil, err
	}
	return &natsCache{kv: kv}, nil
}

func (c *natsCache) Get(url string) (*fetch.Page, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), getTimeout)
	defer cancel()

	entry, err := c.kv.Get(ctx, key(url))
	if err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			log.Printf("vvaves: page cache get: %v", err)
		}
		return nil, false
	}
	var p fetch.Page
	if err := json.Unmarshal(entry.Value(), &p); err != nil {
		log.Printf("vvaves: page cache decode: %v", err)
		return nil, false
	}
	return &p, true
}

func (c *natsCache) Set(url string, p *fetch.Page) {
	ctx, cancel := context.WithTimeout(context.Background(), setTimeout)
	defer cancel()

	value, err := json.Marshal(p)
	if err != nil {
		log.Printf("vvaves: page cache encode: %v", err)
		return
	}
	if _, err := c.kv.Put(ctx, key(url), value); err != nil {
		log.Printf("vvaves: page cache put: %v", err)
	}
}

// key hashes the URL: a KV key is limited to a few safe characters, which a
// URL's scheme separator and query string are not.
func key(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:])
}
