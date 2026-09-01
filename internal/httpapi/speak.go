package httpapi

import (
	"log"
	"net/http"
	"strconv"

	"github.com/lalternative/packages/go/tts"
)

type speakRequest struct {
	Text   string `json:"text"`
	Stream bool   `json:"stream"`
}

func handleSpeak(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Voice == nil {
			writeError(w, http.StatusServiceUnavailable, "speech is not configured")
			return
		}

		var req speakRequest
		if err := decode(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "text is required")
			return
		}

		if req.Stream {
			speakStream(w, r, d, req.Text)
			return
		}

		audio, mime, err := d.Voice.Speak(r.Context(), req.Text)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		// Content-Length matters: without it browsers infer the duration from
		// the first frame header and stop playback at the first seam, so a long
		// text plays only its opening seconds with nothing reported as wrong.
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
		w.WriteHeader(http.StatusOK)
		w.Write(audio)
	}
}

// speakStream emits each piece as it is ready, so listening starts on the
// first one instead of after the last.
//
// Once a piece has been written the 200 is committed and a later failure can
// no longer be reported as a 502 — the response is cut short instead, which is
// the only honest outcome in a chunked body, and the reason streaming is not
// the default.
func speakStream(w http.ResponseWriter, r *http.Request, d Deps, text string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is unsupported")
		return
	}

	// SpeakStream only reports the MIME type when it returns, which is after
	// the first piece has to be written, so the header comes from the
	// configured format instead.
	mime := tts.MIMEFor(d.VoiceFormat)
	sent := 0
	_, err := d.Voice.SpeakStream(r.Context(), text, func(audio []byte) error {
		if sent == 0 {
			w.Header().Set("Content-Type", mime)
			w.WriteHeader(http.StatusOK)
		}
		n, err := w.Write(audio)
		sent += n
		if err != nil {
			// Nobody is listening any more; returning the error stops the
			// library paying for the pieces still to come.
			return err
		}
		flusher.Flush()
		return nil
	})
	if err == nil {
		return
	}
	if sent == 0 {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("tornade: speak stream cut short after %d bytes: %v", sent, err)
}
