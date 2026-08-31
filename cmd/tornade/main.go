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

	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/search/brave"
	"github.com/lalternative/packages/go/search/fetch"
	"github.com/lalternative/packages/go/search/searxng"
	"github.com/lalternative/packages/go/tts"

	"github.com/lalternativefabrique/tornade/internal/config"
	"github.com/lalternativefabrique/tornade/internal/httpapi"
	"github.com/lalternativefabrique/tornade/internal/render"
)

func main() {
	cfg := config.Load()

	browser := render.New(cfg.ChromiumPath)
	defer browser.Close()

	deps := httpapi.Deps{
		Providers:        buildProviders(cfg),
		Renderer:         browser,
		Cache:            fetch.NewMemoryCache(cfg.FetchCacheTTL),
		Voice:            buildVoice(cfg),
		VoiceFormat:      cfg.TTSFormat,
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

// buildVoice returns nil when no speech service is configured, which the
// speak handler reports rather than pretending to a disabled mode.
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
