package health

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A flushed first chunk must arrive before the handler finishes; buffering media
// through TimeoutHandler would block this request and replace its content type.
func TestMediaRouteStreamsBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	complete := make(chan struct{})
	register := func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/media/{id}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Context().Deadline(); !ok {
				t.Error("stream context has no deadline")
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("abc"))
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Error("stream flush unsupported")
			}
			select {
			case <-release:
				_, _ = w.Write([]byte("def"))
			case <-r.Context().Done():
			}
			close(complete)
		})
	}
	router := NewHandler(readinessStub{}, "test").RouterWithMedia(slog.New(slog.NewTextHandler(io.Discard, nil)), 20*time.Millisecond, time.Second, time.Second, register)
	server := httptest.NewServer(router)
	defer server.Close()
	defer close(release)
	client := &http.Client{Timeout: 2 * time.Second}
	res, err := client.Get(server.URL + "/api/v1/media/example")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || res.Header.Get("Content-Type") != "video/mp4" {
		t.Fatalf("media was not delivered directly: status=%d type=%s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	prefix := make([]byte, 3)
	if _, err := io.ReadFull(res.Body, prefix); err != nil || string(prefix) != "abc" {
		t.Fatal("flushed media prefix not received")
	}
	select {
	case <-complete:
		t.Fatal("response was buffered until completion")
	default:
	}
}
