// Package config reads tornade's settings from the environment.
package config

import (
	"os"
	"strconv"
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
		// hosted endpoint, so the hosted default reads a whole page as one
		// utterance while most of the workers sit idle.
		TTSMaxChars:    envInt("TTS_MAX_CHARS", 1000),
		TTSConcurrency: envInt("TTS_CONCURRENCY", 4),
	}
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
