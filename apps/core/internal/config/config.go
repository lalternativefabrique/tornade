// Package config reads tornade's settings from the environment.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr string

	SearxngURL  string
	BraveAPIKey string

	// FetchProxy is the residential endpoint page fetches go through, as
	// "scheme://user:pass@host:port". Publishers behind bot management refuse
	// a datacenter address whatever headers it carries, so without one /fetch
	// is refused by a growing share of the web it exists to read. Empty
	// fetches direct, which is what a local run wants.
	FetchProxy string

	SearchDeadline time.Duration
	FetchCacheTTL  time.Duration

	ChromiumPath     string
	RenderMaxTimeout time.Duration

	TTSURL         string
	TTSAPIKey      string
	TTSModel       string
	TTSVoice       string
	TTSFormat      string
	TTSMaxChars    int
	TTSConcurrency int

	AudioOpeningChars int

	// Keys are the applications' keys read from SPEAK_KEYS as "issuer:key"
	// pairs, an issuer repeatable. One key does both jobs: presented on
	// X-Tornade-Key by the application's server, and the root the signatures
	// on its browser-bound /speak URLs derive from. Empty admits nobody the
	// registry does not, which is what a deployment reachable only from the
	// cluster wants.
	Keys map[string][]string

	// DatabaseURL opens the registry of applications and the admin's own
	// accounts. Empty runs without either: the environment pairs above are
	// then all that speaks, as before the registry existed.
	DatabaseURL string
	// JWTSecret verifies the admin web app's tokens on the admin API. It is
	// the same value the web app mints with.
	JWTSecret string
	// OIDCIssuerURL is the suite's identity provider, read from
	// OIDC_ISSUER_URL. A service presents a bearer token it obtained there
	// instead of an app key; the token must name OIDCAudience. Empty accepts
	// no token.
	OIDCIssuerURL string
	// OIDCAudience is the name this tornade answers to in a token's aud,
	// read from OIDC_AUDIENCE, "tornade" by default.
	OIDCAudience string
	// SpeakUnguarded lets the speak routes answer with no key at all, read
	// from SPEAK_UNGUARDED=true. For a tornade nothing outside the cluster
	// reaches, and for a laptop; never for one behind a public name.
	SpeakUnguarded bool
	// RegistryEncryptionKey seals the applications' keys at rest, a base64
	// 32-byte key. Required with a database: a registry that stores secrets
	// in the clear is one that must not start.
	RegistryEncryptionKey string
}

func Load() Config {
	return Config{
		Addr: env("LISTEN_ADDR", ":8080"),

		SearxngURL:  os.Getenv("SEARXNG_URL"),
		BraveAPIKey: os.Getenv("BRAVE_API_KEY"),
		FetchProxy:  os.Getenv("FETCH_PROXY"),

		SearchDeadline: envDuration("SEARCH_DEADLINE_MS", 4*time.Second),
		FetchCacheTTL:  envDuration("FETCH_CACHE_TTL_MS", 15*time.Minute),

		ChromiumPath:     os.Getenv("CHROMIUM_PATH"),
		RenderMaxTimeout: envDuration("RENDER_MAX_TIMEOUT_MS", 20*time.Second),

		TTSURL:    os.Getenv("PIPER_URL"),
		TTSAPIKey: os.Getenv("TTS_API_KEY"),
		TTSModel:  os.Getenv("TTS_MODEL"),
		TTSVoice:  os.Getenv("TTS_VOICE"),
		TTSFormat: env("TTS_FORMAT", "mp3"),
		// Piper has no per-request limit and is slower per character than a
		// hosted endpoint, so the hosted default would read a whole page as a
		// single utterance and nothing could be streamed until it was done.
		TTSMaxChars: envInt("TTS_MAX_CHARS", 120),
		// A self-hosted Piper serializes synthesis — one utterance at a time
		// per process — so concurrent requests queue rather than overlap, and
		// each one only delays the piece the listener is waiting for. Measured
		// against it, concurrency 1 returns first audio in 2.4s where 4 takes
		// 3.9s, for 6% more total time. Raise it only for a backend that
		// actually synthesizes in parallel.
		TTSConcurrency: envInt("TTS_CONCURRENCY", 1),

		// Not TTSMaxChars, though both are a number of characters. That one is
		// how small a reading is cut for Piper to work on; this is how much of
		// a text counts as its opening, and the primer and the reader must
		// agree on it exactly — they each split the text themselves, and a
		// disagreement has the two halves meet somewhere other than the same
		// cut, reading a word twice or skipping one.
		AudioOpeningChars: envInt("AUDIO_OPENING_CHARS", 800),

		Keys: envPairs("SPEAK_KEYS"),

		DatabaseURL:           os.Getenv("DATABASE_URL"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		RegistryEncryptionKey: os.Getenv("REGISTRY_ENCRYPTION_KEY"),
		SpeakUnguarded:        os.Getenv("SPEAK_UNGUARDED") == "true",
		OIDCIssuerURL:         os.Getenv("OIDC_ISSUER_URL"),
		OIDCAudience:          envString("OIDC_AUDIENCE", "tornade"),
	}
}

// envPairs reads "issuer:key,issuer:key" into a map. A malformed entry
// is dropped rather than guessed at: a key read wrong is a key that rejects
// every signature made with it, and silence about it would look like the
// application signing incorrectly.
func envPairs(key string) map[string][]string {
	out := map[string][]string{}
	for _, entry := range strings.Split(os.Getenv(key), ",") {
		issuer, secret, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || issuer == "" || secret == "" {
			continue
		}
		out[issuer] = append(out[issuer], secret)
	}
	return out
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if ms, err := strconv.Atoi(os.Getenv(key)); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return fallback
}
