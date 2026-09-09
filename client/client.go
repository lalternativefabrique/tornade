package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lalternative/packages/go/audioreader"
	"github.com/lalternative/packages/go/tts"
)

// Config points a voice at a tornade.
type Config struct {
	BaseURL string
	// Scope names where tornade keeps the readings, so two applications
	// sharing one tornade do not overwrite each other's cache. Empty leaves
	// the naming to tornade's default.
	Scope string
	// AppKey authenticates server-to-server calls on a tornade reachable from
	// the internet, where a signature only ever buys one listen. Empty sends
	// nothing, which an internal-only tornade accepts.
	AppKey string
	// Client defaults to one with no global timeout, same as OpenAIVoice: a
	// long reading can take minutes, and cancellation belongs to the context.
	Client *http.Client
}

// HeaderAppKey carries AppKey; the server reads the same name.
const HeaderAppKey = "X-Tornade-Key"

// Voice reads text through tornade instead of a speech service directly.
// Tornade owns the synthesis, the cache and the store, so every application
// speaking through the same tornade shares one paid reading of the same
// words, and can prime or pregenerate one ahead of any listener.
type Voice struct {
	cfg Config
}

var _ tts.Voice = (*Voice)(nil)

// New wires a voice against tornade, or nil without a BaseURL: absent, not
// half-present, so a caller checks for nil and runs without audio.
func New(cfg Config) *Voice {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Client == nil {
		cfg.Client = &http.Client{}
	}
	return &Voice{cfg: cfg}
}

func (v *Voice) Speak(ctx context.Context, text string) ([]byte, string, error) {
	return v.SpeakNamed(ctx, "", text)
}

// SpeakNamed reads text under a name, so a reading primed or pregenerated
// earlier under that same name is the one served. An empty id has tornade key
// the reading on the text alone, which is what Speak does.
func (v *Voice) SpeakNamed(ctx context.Context, id, text string) ([]byte, string, error) {
	resp, err := v.speak(ctx, id, text, false)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("tts: read body: %w", err)
	}
	if len(audio) == 0 {
		return nil, "", fmt.Errorf("tts: no audio for a %d-rune text", len([]rune(text)))
	}
	return audio, respMIME(resp), nil
}

func (v *Voice) SpeakStream(ctx context.Context, text string, emit func([]byte) error) (string, error) {
	return v.SpeakStreamNamed(ctx, "", text, emit)
}

// SpeakStreamNamed streams text under a name. It is the call that turns a
// primed opening into something heard: tornade only serves an opening read
// ahead of time on the streaming path, so a caller that primed and then asks
// whole pays for the opening twice and waits for all of it.
func (v *Voice) SpeakStreamNamed(ctx context.Context, id, text string, emit func([]byte) error) (string, error) {
	resp, err := v.speak(ctx, id, text, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// A cache hit is served whole with its real content type even when the
	// request asked to stream — tornade answers from the store before it
	// considers reading anything aloud. Only a paying listen carries frames.
	if resp.Header.Get("Content-Type") != audioreader.FramesContentType {
		audio, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("tts: read body: %w", err)
		}
		if len(audio) == 0 {
			return "", fmt.Errorf("tts: no audio for a %d-rune text", len([]rune(text)))
		}
		if err := emit(audio); err != nil {
			return "", err
		}
		return respMIME(resp), nil
	}

	var got bool
	for {
		var length [4]byte
		if _, err := io.ReadFull(resp.Body, length[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("tts: read frame length: %w", err)
		}
		piece := make([]byte, binary.BigEndian.Uint32(length[:]))
		if _, err := io.ReadFull(resp.Body, piece); err != nil {
			return "", fmt.Errorf("tts: read frame: %w", err)
		}
		if len(piece) == 0 {
			continue
		}
		got = true
		if err := emit(piece); err != nil {
			return "", err
		}
	}
	if !got {
		return "", fmt.Errorf("tts: no audio for a %d-rune text", len([]rune(text)))
	}
	return tts.MIMEFor("mp3"), nil
}

// Pregenerate asks tornade to read text in full and keep it, ahead of any
// listener. Tornade acknowledges before the reading starts, so a nil return
// means scheduled, not stored; id is what lets the reading be asked for later
// under the same name.
func (v *Voice) Pregenerate(ctx context.Context, id, text string) error {
	resp, err := v.post(ctx, "/speak/pregenerate", map[string]any{
		"text":  text,
		"scope": v.cfg.Scope,
		"id":    id,
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// PrimeOpening asks tornade to read only the start of text and keep it, ahead
// of any listener: one request that buys the seconds before play, where
// Pregenerate pays for the whole text on the chance that someone listens.
// Tornade acknowledges before the reading starts, so a nil return means
// scheduled, not stored. id is required: an opening nobody can name again is
// one no listener will ever be served.
func (v *Voice) PrimeOpening(ctx context.Context, id, text string) error {
	resp, err := v.post(ctx, "/speak/prime", map[string]any{
		"text":  text,
		"scope": v.cfg.Scope,
		"id":    id,
	})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (v *Voice) speak(ctx context.Context, id, text string, stream bool) (*http.Response, error) {
	payload := map[string]any{
		"text":   text,
		"scope":  v.cfg.Scope,
		"stream": stream,
	}
	if id != "" {
		payload["id"] = id
	}
	return v.post(ctx, "/speak", payload)
}

func (v *Voice) post(ctx context.Context, path string, payload map[string]any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("tts: build request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.cfg.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tts: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if v.cfg.AppKey != "" {
		req.Header.Set(HeaderAppKey, v.cfg.AppKey)
	}
	resp, err := v.cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts: call: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("tts: status %d: %s", resp.StatusCode, bytes.TrimSpace(detail))
	}
	return resp, nil
}

func respMIME(resp *http.Response) string {
	ct := resp.Header.Get("Content-Type")
	if ct == "" || strings.HasPrefix(ct, "text/") || strings.HasPrefix(ct, "application/json") {
		return tts.MIMEFor("mp3")
	}
	return ct
}
