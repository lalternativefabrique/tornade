package signed

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Signer hands a browser a URL onto tornade's /speak for one reading.
//
// The application decides who may hear what, signs that one reading, and the
// bytes go straight from tornade to the browser: relaying tens of seconds of
// audio through the application's own server is what this avoids.
type Signer struct {
	publicURL string
	issuer    string
	key       string
	ttl       time.Duration
}

// SignerConfig points at the tornade a browser fetches audio from.
type SignerConfig struct {
	// PublicURL is the origin the browser reaches tornade on, which is not
	// the address the application calls it on from inside the cluster.
	PublicURL string
	// Issuer names this application in the signature, so tornade knows which
	// secret to check it against.
	Issuer string
	// Key is the application's tornade key, the one it presents on its own
	// calls too; the signing key is derived from it.
	Key string
	// TTL is how long a handed-out URL stays valid: long enough to press play
	// on a reply that has been sitting on screen, short enough that a link
	// copied out of the network tab stops working. Defaults to 30 minutes.
	TTL time.Duration
}

const defaultTTL = 30 * time.Minute

// NewSigner returns nil without a public URL, an issuer or a key: an unsigned
// URL is one tornade refuses, so handing one out would only produce a player
// that fails on every press.
func NewSigner(cfg SignerConfig) *Signer {
	public := strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if public == "" || strings.TrimSpace(cfg.Issuer) == "" || strings.TrimSpace(cfg.Key) == "" {
		return nil
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Signer{publicURL: public, issuer: cfg.Issuer, key: cfg.Key, ttl: ttl}
}

// URL returns where the browser may fetch this reading, and until when.
//
// The signature covers the text, so the URL authorises this reading and
// nothing else: it cannot be spent having some other text synthesized.
func (s *Signer) URL(scope, id, text string) (string, time.Time) {
	expires := time.Now().Add(s.ttl)
	q := Sign(s.issuer, s.key, Params{
		Scope: scope, ID: id, TextHash: HashText(text), Expires: expires,
	})
	return s.publicURL + "/speak?" + q.Encode(), expires
}

// PublicOrigin is where the browser fetches audio, for a caller that needs to
// name it: a CSP's connect-src, say.
func (s *Signer) PublicOrigin() string {
	if s == nil {
		return ""
	}
	if u, err := url.Parse(s.publicURL); err == nil && u.Host != "" {
		return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}
	return s.publicURL
}
