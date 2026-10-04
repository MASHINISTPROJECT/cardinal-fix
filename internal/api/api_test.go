//go:build linux

package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cardinal/internal/container"
	"cardinal/internal/state"
)

// syncRecorder is a goroutine-safe http.ResponseWriter for tests that read
// the body while the handler is still writing. httptest.ResponseRecorder
// is not safe for concurrent Write + Body access under -race.
type syncRecorder struct {
	mu      sync.Mutex
	header  http.Header
	body    bytes.Buffer
	code    int
	flushed bool
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{header: make(http.Header)}
}

func (s *syncRecorder) Header() http.Header { return s.header }

func (s *syncRecorder) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body.Write(b)
}

func (s *syncRecorder) WriteHeader(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.code = code
}

func (s *syncRecorder) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushed = true
}

func (s *syncRecorder) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.body.String()
}

func TestHandleContainerExec(t *testing.T) {
	c := &container.Container{ID: "abc", Name: "web"}

	t.Run("GET returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/containers/abc/exec", nil)
		rec := httptest.NewRecorder()
		handleContainerExec(rec, req, c)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("empty Cmd returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/containers/abc/exec", strings.NewReader(`{"Cmd":[]}`))
		rec := httptest.NewRecorder()
		handleContainerExec(rec, req, c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("Tty returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/containers/abc/exec", strings.NewReader(`{"Cmd":["echo","hi"],"Tty":true}`))
		rec := httptest.NewRecorder()
		handleContainerExec(rec, req, c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
		if body := rec.Body.String(); !strings.Contains(body, "TTY") {
			t.Fatalf("body %q does not contain TTY", body)
		}
	})
}

func TestHandleEventsSSE(t *testing.T) {
	t.Setenv("CARDINAL_DATA_DIR", t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleEvents(rec, req)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for {
		container.EmitEvent(container.EventStart, &container.Container{ID: "a", Name: "w"})
		time.Sleep(50 * time.Millisecond)
		if body := rec.snapshot(); strings.Contains(body, "data:") && strings.Contains(body, `"type"`) {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("timed out waiting for SSE frame, body %q", rec.snapshot())
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("handleEvents did not return after context cancel")
	}

	body := rec.snapshot()
	if !strings.Contains(body, "data:") {
		t.Fatalf("body %q does not contain data:", body)
	}
	if !strings.Contains(body, `"type"`) {
		t.Fatalf("body %q does not contain type", body)
	}
}

func TestHandleContainerLogsFollow(t *testing.T) {
	t.Setenv("CARDINAL_DATA_DIR", t.TempDir())

	c := &container.Container{ID: "w", Name: "w"}
	logPath := state.LogPath(c.ID)
	if err := os.MkdirAll(state.LogsDir(), 0700); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(logPath, []byte("chunk1\n"), 0644); err != nil {
		t.Fatalf("write initial logs: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/containers/w/logs?follow=1", nil).WithContext(ctx)
	rec := newSyncRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handleContainerLogs(rec, req, c)
	}()

	time.Sleep(200 * time.Millisecond)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		cancel()
		<-done
		t.Fatalf("open log for append: %v", err)
	}
	if _, err := f.WriteString("chunk2\n"); err != nil {
		_ = f.Close()
		cancel()
		<-done
		t.Fatalf("append logs: %v", err)
	}
	_ = f.Close()

	time.Sleep(1500 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("handleContainerLogs did not return after context cancel")
	}

	body := rec.snapshot()
	if !strings.Contains(body, "chunk1") {
		t.Fatalf("body %q does not contain chunk1", body)
	}
	if !strings.Contains(body, "chunk2") {
		t.Fatalf("body %q does not contain chunk2", body)
	}
}
