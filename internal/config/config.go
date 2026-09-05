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
	// with, read from SPEAK_SIGNING_KEYS as "issuer:secret" pairs. Empty
	// accepts no browser call, which is what a deployment reachable only from
	// the cluster wants.
	SigningKeys map[string]string
	// AppKeys authenticate a service calling /speak on its own behalf, read
	// from SPEAK_APP_KEYS in the same "issuer:secret" shape as SigningKeys —
	// one format for both rather than two that look alike and are not. Keyed
	// by secret here, since that is what a request presents; the name it maps
	// to is what the logs report.
	AppKeys map[string]string
}

func Load() Config {
	return Config{
		Addr: env("LISTEN_ADDR", ":8080"),

		SearxngURL:  os.Getenv("SEARXNG_URL"),
		BraveAPIKey: os.Getenv("BRAVE_API_KEY"),

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
		AppKeys:     bySecret(envPairs("SPEAK_APP_KEYS")),
	}
}

// envPairs reads "issuer:secret,issuer:secret" into a map. A malformed entry
// is dropped rather than guessed at: a key read wrong is a key that rejects
// every signature made with it, and silence about it would look like the
// application signing incorrectly.
func envPairs(key string) map[string]string {
	out := map[string]string{}
	for _, entry := range strings.Split(os.Getenv(key), ",") {
		issuer, secret, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || issuer == "" || secret == "" {
			continue
		}
		out[issuer] = secret
	}
	return out
}

// bySecret inverts an issuer-to-secret map, since a request presents the
// secret and what is wanted from it is the name.
func bySecret(pairs map[string]string) map[string]string {
	out := make(map[string]string, len(pairs))
	for issuer, secret := range pairs {
		out[secret] = issuer
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
