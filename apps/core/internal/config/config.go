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

	// SigningKeys are the secrets applications sign browser-bound /speak URLs
	// with, read from SPEAK_SIGNING_KEYS as "issuer:secret" pairs. An issuer
	// may appear more than once. Empty accepts no browser call, which is
	// what a deployment reachable only from the cluster wants.
	SigningKeys map[string][]string
	// AppKeys authenticate a service calling /speak on its own behalf, read
	// from SPEAK_APP_KEYS in the same "issuer:secret" shape as SigningKeys —
	// one format for both rather than two that look alike and are not.
	AppKeys map[string][]string

	// DatabaseURL opens the registry of applications and the admin's own
	// accounts. Empty runs without either: the environment pairs above are
	// then all that speaks, as before the registry existed.
	DatabaseURL string
	// JWTSecret verifies the admin web app's tokens on the admin API. It is
	// the same value the web app mints with.
	JWTSecret string
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

		SigningKeys: envPairs("SPEAK_SIGNING_KEYS"),
		AppKeys:     envPairs("SPEAK_APP_KEYS"),

		DatabaseURL:           os.Getenv("DATABASE_URL"),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		RegistryEncryptionKey: os.Getenv("REGISTRY_ENCRYPTION_KEY"),
	}
}

// envPairs reads "issuer:secret,issuer:secret" into a map. A malformed entry
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
