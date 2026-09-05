// Package signed authenticates a /speak request that came straight from a
// browser rather than from an application's own server.
//
// The point of the direct call is that the audio does not pass through the
// application: a reading is tens of seconds of bytes, and relaying it makes
// the caller's own server stream media for the whole of it. But a browser
// cannot be trusted with a credential, so it is handed a signature instead —
// one that covers a single reading and expires on its own.
//
// This is the S3 presigned-URL shape, for the same reason: the service
// holding the bytes serves them, and the service holding the rights only
// authorises. Tornade still knows nothing about users — it checks a MAC over
// what was asked for, and that is the whole of its notion of identity.
package signed

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Params are the fields a signature covers.
//
// The text is covered by its hash rather than in full: a URL carrying a whole
// article would not survive any proxy's length limit, and the hash is what
// names the reading in the store anyway.
type Params struct {
	Scope    string
	ID       string
	TextHash string
	Expires  time.Time
}

// Verifier checks signatures against the keys the applications sign with.
//
// Keyed by an issuer name the caller sends alongside the signature, so each
// application has its own secret and one being rotated or revoked leaves the
// others alone. No issuer configured disables signed access entirely, which
// is what an internal-only deployment wants: nothing reaches /speak that did
// not already reach the cluster.
type Verifier struct {
	keys map[string][]byte
	now  func() time.Time
}

// NewVerifier returns nil when no key is configured, so a deployment that
// never meant to accept browser calls rejects them by not having the check at
// all rather than by having one nobody set up.
func NewVerifier(keys map[string]string) *Verifier {
	parsed := make(map[string][]byte, len(keys))
	for issuer, key := range keys {
		if strings.TrimSpace(issuer) == "" || key == "" {
			continue
		}
		parsed[issuer] = []byte(key)
	}
	if len(parsed) == 0 {
		return nil
	}
	return &Verifier{keys: parsed, now: time.Now}
}

// Errors a caller distinguishes: an expired link is worth telling a listener
// about, since asking again fixes it, while a bad signature is not.
var (
	ErrNoSignature  = fmt.Errorf("signed: no signature")
	ErrUnknownIssue = fmt.Errorf("signed: unknown issuer")
	ErrExpired      = fmt.Errorf("signed: signature expired")
	ErrBadSignature = fmt.Errorf("signed: signature does not match")
)

// Query names the parameters a signed URL carries.
const (
	QueryIssuer    = "iss"
	QueryExpires   = "exp"
	QuerySignature = "sig"
)

// Verify checks the signature in q against what it claims to authorise.
//
// scope, id and text come from the request body — the signature is only worth
// anything if it covers what is actually being asked for, not what the URL
// says is being asked for.
func (v *Verifier) Verify(q url.Values, scope, id, text string) error {
	if v == nil {
		return ErrNoSignature
	}
	sig := q.Get(QuerySignature)
	if sig == "" {
		return ErrNoSignature
	}
	key, ok := v.keys[q.Get(QueryIssuer)]
	if !ok {
		return ErrUnknownIssue
	}

	unix, err := strconv.ParseInt(q.Get(QueryExpires), 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	expires := time.Unix(unix, 0)

	want := sign(key, Params{Scope: scope, ID: id, TextHash: HashText(text), Expires: expires})
	// Constant time: a byte-by-byte comparison leaks how much of a guess was
	// right, which is enough to find the rest one byte at a time.
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return ErrBadSignature
	}
	// Checked after the MAC, so an expired link and a forged one take the
	// same path until the signature itself is known to be genuine.
	if v.now().After(expires) {
		return ErrExpired
	}
	return nil
}

// Sign builds the query parameters authorising one reading until expires.
//
// Exported so the applications that hand out these URLs sign them with this
// code rather than reimplementing the construction — a signature scheme
// written twice is a signature scheme with two behaviours.
func Sign(issuer, key string, p Params) url.Values {
	q := url.Values{}
	q.Set(QueryIssuer, issuer)
	q.Set(QueryExpires, strconv.FormatInt(p.Expires.Unix(), 10))
	q.Set(QuerySignature, sign([]byte(key), p))
	return q
}

// HashText names a text the way the signature covers it.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// sign builds the MAC over the canonical form of p.
//
// The fields are joined with a separator that cannot appear in the values
// being joined, so no two different sets of parameters can produce the same
// string to sign — "a" + "bc" and "ab" + "c" must not collide.
func sign(key []byte, p Params) string {
	fields := []string{
		"scope=" + p.Scope,
		"id=" + p.ID,
		"text=" + p.TextHash,
		"exp=" + strconv.FormatInt(p.Expires.Unix(), 10),
	}
	sort.Strings(fields)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(strings.Join(fields, "\n")))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
