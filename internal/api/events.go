//go:build linux

package api

import (
	"encoding/json"
	"net/http"
	"time"

	"cardinal/internal/container"
)

func handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, 405, "method not allowed")
		return
	}
	since := time.Time{}
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
	for _, evt := range container.EventsSince(since) { // replay
		if err := writeSSE(w, evt); err != nil {
			return
		}
	}
	flusher.Flush()
	ch := container.SubscribeEvents(100)
	defer container.UnsubscribeEvents(ch)
	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			if err := writeSSE(w, evt); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, evt container.Event) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte("data: " + string(data) + "\n\n"))
	return err
}
