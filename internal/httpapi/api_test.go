package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/tts"

	"github.com/lalternativefabrique/tornade/internal/httpapi"
)

type stubProvider struct {
	results  []search.Result
	err      error
	gotQuery search.Query
}

func (s *stubProvider) Search(_ context.Context, q search.Query) (*search.Response, error) {
	s.gotQuery = q
	if s.err != nil {
		return nil, s.err
	}
	return &search.Response{Query: q.Text, Results: s.results}, nil
}

type stubRenderer struct {
	html     string
	finalURL string
	err      error
	gotTimet time.Duration
}

func (s *stubRenderer) Render(ctx context.Context, url string, t time.Duration) (string, error) {
	html, _, err := s.RenderPage(ctx, url, t)
	return html, err
}

func (s *stubRenderer) RenderPage(_ context.Context, _ string, t time.Duration) (string, string, error) {
	s.gotTimet = t
	if s.err != nil {
		return "", "", s.err
	}
	return s.html, s.finalURL, nil
}

type stubVoice struct {
	pieces [][]byte
	err    error
}

func (v *stubVoice) Speak(ctx context.Context, text string) ([]byte, string, error) {
	var out []byte
	mime, err := v.SpeakStream(ctx, text, func(a []byte) error {
		out = append(out, a...)
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return out, mime, nil
}

func (v *stubVoice) SpeakStream(_ context.Context, _ string, emit func([]byte) error) (string, error) {
	for _, p := range v.pieces {
		if err := emit(p); err != nil {
			return "", err
		}
	}
	if v.err != nil {
		return "", v.err
	}
	return tts.MIMEFor("mp3"), nil
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func baseDeps() httpapi.Deps {
	return httpapi.Deps{
		SearchDeadline:   4 * time.Second,
		RenderMaxTimeout: 20 * time.Second,
		VoiceFormat:      "mp3",
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	httpapi.New(baseDeps()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
}

func TestSearchDefaultsToGeneral(t *testing.T) {
	p := &stubProvider{results: []search.Result{{Title: "hit", URL: "https://e.com"}}}
	d := baseDeps()
	d.Providers = map[search.Category]search.Provider{search.CategoryGeneral: p}

	rec := post(t, httpapi.New(d), "/search", `{"q":"gramsci"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if p.gotQuery.Category != search.CategoryGeneral {
		t.Errorf("category = %q, want general", p.gotQuery.Category)
	}
	var out struct {
		Results []map[string]any `json:"results"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Results) != 1 {
		t.Fatalf("results = %+v", out.Results)
	}
	// The library's structs carry no JSON tags, so the wire contract has to be
	// restated rather than serialised straight through.
	if out.Results[0]["title"] != "hit" || out.Results[0]["url"] != "https://e.com" {
		t.Errorf("result keys = %+v", out.Results[0])
	}
}

func TestSearchRejectsUnknownCategory(t *testing.T) {
	d := baseDeps()
	d.Providers = map[search.Category]search.Provider{search.CategoryGeneral: &stubProvider{}}

	if rec := post(t, httpapi.New(d), "/search", `{"q":"x","categories":["images"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestSearchWithoutProvidersIs503(t *testing.T) {
	if rec := post(t, httpapi.New(baseDeps()), "/search", `{"q":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}

func TestSearchSurfacesBackendFailureAs502(t *testing.T) {
	d := baseDeps()
	d.Providers = map[search.Category]search.Provider{
		search.CategoryGeneral: &stubProvider{err: errors.New("searxng down")},
	}
	if rec := post(t, httpapi.New(d), "/search", `{"q":"x"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestSearchMergesCategories(t *testing.T) {
	general := &stubProvider{results: []search.Result{{Title: "web", URL: "https://a.com"}}}
	academic := &stubProvider{results: []search.Result{{Title: "paper", URL: "https://b.com"}}}
	d := baseDeps()
	d.Providers = map[search.Category]search.Provider{
		search.CategoryGeneral:  general,
		search.CategoryAcademic: academic,
	}

	rec := post(t, httpapi.New(d), "/search",
		`{"q":"gramsci","categories":["general","academic"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		Results []map[string]any `json:"results"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Results) != 2 {
		t.Fatalf("want both lists fused, got %+v", out.Results)
	}
	// Each provider must be asked for its own category, not the caller's first.
	if academic.gotQuery.Category != search.CategoryAcademic {
		t.Errorf("academic provider queried with %q", academic.gotQuery.Category)
	}
}

func TestRenderClampsTimeout(t *testing.T) {
	r := &stubRenderer{html: "<html></html>", finalURL: "https://e.com/"}
	d := baseDeps()
	d.Renderer = r

	rec := post(t, httpapi.New(d), "/render", `{"url":"https://e.com","timeout_ms":900000}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if r.gotTimet != 20*time.Second {
		t.Errorf("timeout = %s, want clamped to 20s", r.gotTimet)
	}
}

func TestRenderRejectsNonHTTPURL(t *testing.T) {
	d := baseDeps()
	d.Renderer = &stubRenderer{}
	if rec := post(t, httpapi.New(d), "/render", `{"url":"file:///etc/passwd"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestRenderSurfacesFailureAs502(t *testing.T) {
	d := baseDeps()
	d.Renderer = &stubRenderer{err: errors.New("navigation timed out")}
	if rec := post(t, httpapi.New(d), "/render", `{"url":"https://e.com"}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestSpeakSendsContentLength(t *testing.T) {
	d := baseDeps()
	d.Voice = &stubVoice{pieces: [][]byte{[]byte("aaa"), []byte("bb")}}

	rec := post(t, httpapi.New(d), "/speak", `{"text":"bonjour"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	// Without a declared length browsers infer the duration from the first
	// frame header and stop at the first seam.
	if got := rec.Header().Get("Content-Length"); got != "5" {
		t.Errorf("Content-Length = %q, want 5", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestSpeakWithoutVoiceIs503(t *testing.T) {
	if rec := post(t, httpapi.New(baseDeps()), "/speak", `{"text":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503", rec.Code)
	}
}

func TestSpeakRequiresText(t *testing.T) {
	d := baseDeps()
	d.Voice = &stubVoice{}
	if rec := post(t, httpapi.New(d), "/speak", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}

func TestSpeakStreamEmitsPieces(t *testing.T) {
	d := baseDeps()
	d.Voice = &stubVoice{pieces: [][]byte{[]byte("one"), []byte("two")}}

	rec := post(t, httpapi.New(d), "/speak", `{"text":"x","stream":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	if rec.Body.String() != "onetwo" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Error("a streamed response must not declare a length")
	}
}

func TestSpeakStreamFailingBeforeFirstPieceIs502(t *testing.T) {
	d := baseDeps()
	d.Voice = &stubVoice{err: errors.New("piper down")}
	if rec := post(t, httpapi.New(d), "/speak", `{"text":"x","stream":true}`); rec.Code != http.StatusBadGateway {
		t.Fatalf("got %d, want 502", rec.Code)
	}
}

func TestInvalidJSONIs400(t *testing.T) {
	d := baseDeps()
	d.Providers = map[search.Category]search.Provider{search.CategoryGeneral: &stubProvider{}}
	if rec := post(t, httpapi.New(d), "/search", `{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", rec.Code)
	}
}
