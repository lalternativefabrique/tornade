package signed

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewSignerNeedsAURLAnIssuerAndAKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  SignerConfig
	}{
		{"nothing", SignerConfig{}},
		{"no key", SignerConfig{PublicURL: "https://audio.example", Issuer: issuer}},
		{"no url", SignerConfig{Issuer: issuer, Key: key}},
		{"no issuer", SignerConfig{PublicURL: "https://audio.example", Key: key}},
		{"blank", SignerConfig{PublicURL: "  ", Issuer: " ", Key: "  "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NewSigner(tc.cfg); got != nil {
				t.Error("NewSigner returned non-nil with nothing to hand out")
			}
		})
	}
}

// The URL is what the server will check, so it has to verify against the
// same secret: this is the contract between the two sides.
func TestSignedURLVerifies(t *testing.T) {
	s := NewSigner(SignerConfig{PublicURL: "https://audio.example/", Issuer: issuer, Key: key})

	raw, expires := s.URL("chat-message", "msg-42", "bonjour")
	if !strings.HasPrefix(raw, "https://audio.example/speak?") {
		t.Fatalf("URL = %q, want it to point at /speak on the public origin", raw)
	}
	if time.Until(expires) <= 0 {
		t.Fatal("the URL expired before it was handed out")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := NewVerifier(map[string]string{issuer: key})
	if err := v.Verify(parsed.Query(), "chat-message", "msg-42", "bonjour"); err != nil {
		t.Fatalf("the server would refuse the URL we handed out: %v", err)
	}
}

func TestSignedURLDoesNotAuthoriseAnotherText(t *testing.T) {
	s := NewSigner(SignerConfig{PublicURL: "https://audio.example", Issuer: issuer, Key: key})

	raw, _ := s.URL("chat-message", "msg-42", "bonjour")
	parsed, _ := url.Parse(raw)

	v := NewVerifier(map[string]string{issuer: key})
	if err := v.Verify(parsed.Query(), "chat-message", "msg-42", "something else"); err == nil {
		t.Fatal("the URL authorised a text it was not signed for")
	}
}

func TestSignedURLExpiresOnTheConfiguredTTL(t *testing.T) {
	s := NewSigner(SignerConfig{PublicURL: "https://audio.example", Issuer: issuer, Key: key, TTL: time.Minute})

	_, expires := s.URL("chat-message", "msg-42", "bonjour")
	if d := time.Until(expires); d > time.Minute+time.Second || d < 50*time.Second {
		t.Errorf("expiry in %v, want about a minute", d)
	}
}

func TestPublicOriginDropsThePath(t *testing.T) {
	s := NewSigner(SignerConfig{PublicURL: "https://audio.example/base", Issuer: issuer, Key: key})
	if got := s.PublicOrigin(); got != "https://audio.example" {
		t.Errorf("PublicOrigin = %q", got)
	}
	var none *Signer
	if got := none.PublicOrigin(); got != "" {
		t.Errorf("nil signer origin = %q, want empty", got)
	}
}
