// Command tornade is the HTTP facade over this platform's search, page
// extraction, JavaScript rendering and speech backends.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lalternative/packages/go/audioreader"
	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/search/brave"
	"github.com/lalternative/packages/go/search/fetch"
	"github.com/lalternative/packages/go/search/searxng"
	"github.com/lalternative/packages/go/tts"

	"github.com/lalternativefabrique/tornade/internal/audio"
	"github.com/lalternativefabrique/tornade/internal/config"
	"github.com/lalternativefabrique/tornade/internal/httpapi"
	"github.com/lalternativefabrique/tornade/internal/render"
)

func main() {
	cfg := config.Load()

	browser := render.New(cfg.ChromiumPath)
	defer browser.Close()

	reader, primer := buildAudio(cfg)

	deps := httpapi.Deps{
		Providers:        buildProviders(cfg),
		Renderer:         browser,
		Cache:            fetch.NewMemoryCache(cfg.FetchCacheTTL),
		Reader:           reader,
		Primer:           primer,
		SearchDeadline:   cfg.SearchDeadline,
		RenderMaxTimeout: cfg.RenderMaxTimeout,
	}

	srv := &http.Server{Addr: cfg.Addr, Handler: httpapi.New(deps)}

	go func() {
		log.Printf("tornade: listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("tornade: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("tornade: shutdown: %v", err)
	}
}

// buildProviders wires one provider per category. Brave has no academic index
// of its own, so it backs up only the general category — where a self-hosted
// SearXNG actually breaks, an upstream engine being rate-limited or blocked.
func buildProviders(cfg config.Config) map[search.Category]search.Provider {
	if cfg.SearxngURL == "" {
		return nil
	}
	providers := map[search.Category]search.Provider{
		search.CategoryGeneral:  searxng.New(cfg.SearxngURL, nil),
		search.CategoryAcademic: searxng.New(cfg.SearxngURL, nil),
	}
	if cfg.BraveAPIKey != "" {
		providers[search.CategoryGeneral] = search.WithFallback(
			providers[search.CategoryGeneral], brave.New(cfg.BraveAPIKey, nil))
	}
	return providers
}

// buildAudio wires the reader that serves readings and the primer that reads
// their openings ahead of time.
//
// Three shapes, and each degrades into the one below it. With a voice and a
// bucket, a reading is kept and its opening can be made before anyone asks.
// With a voice alone, every listen pays for its own reading — worse, but it
// works, and it is what a tornade with no S3 configured has always done.
// With no voice, both are nil and the speak endpoints report themselves
// unconfigured rather than failing at the first byte.
//
// A store that is configured but broken is fatal here rather than silently
// dropped: someone asked for a cache, and starting without one would hide
// that behind a bill nobody notices until it arrives.
func buildAudio(cfg config.Config) (*audioreader.Reader, *audioreader.Primer) {
	provider := audio.NewProvider(buildVoice(cfg))
	if provider == nil {
		return nil, nil
	}

	store, err := audio.NewStoreFromEnv()
	if err != nil {
		log.Fatalf("tornade: %v", err)
	}
	if store == nil {
		log.Print("tornade: no S3 bucket configured, every reading will be paid for")
		return audioreader.NewReader(provider, nil, cfg.AudioOpeningChars, nil), nil
	}

	// The same opening size on both: they each split the text themselves, and
	// two different sizes have the halves meet somewhere other than the same
	// cut — a word read twice, or one skipped.
	return audioreader.NewReader(provider, store, cfg.AudioOpeningChars, nil),
		audioreader.NewPrimer(provider, store, cfg.AudioOpeningChars, nil)
}

// buildVoice returns nil when no speech service is configured, which the
// speak handlers report rather than pretending to a disabled mode.
func buildVoice(cfg config.Config) tts.Voice {
	if cfg.TTSURL == "" {
		return nil
	}
	return tts.NewOpenAIVoice(tts.Config{
		BaseURL:     cfg.TTSURL,
		APIKey:      cfg.TTSAPIKey,
		Model:       cfg.TTSModel,
		VoiceID:     cfg.TTSVoice,
		Format:      cfg.TTSFormat,
		MaxChars:    cfg.TTSMaxChars,
		Concurrency: cfg.TTSConcurrency,
	})
}
