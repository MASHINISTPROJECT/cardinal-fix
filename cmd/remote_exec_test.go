//go:build linux

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it (mirror of captureStderr in exit_test.go).
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// setRemote points the package remote vars at host/token, restoring them
// after the test.
func setRemote(t *testing.T, host, token string) {
	t.Helper()
	oldHost, oldToken := remoteHost, remoteToken
	remoteHost, remoteToken = host, token
	t.Cleanup(func() { remoteHost, remoteToken = oldHost, oldToken })
}

// remoteStub serves the `cardinal serve` endpoints the remote CLI branches
// hit, following the mux style of internal/client/client_test.go.
func remoteStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/web/exec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Id": "abc123_exec", "Output": "hi\n", "Stderr": "", "ExitCode": 3,
		})
	})
	mux.HandleFunc("/containers/web/top", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Titles":    []string{"PID", "CMD"},
			"Processes": []string{"1 init", "2 sh"},
		})
	})
	mux.HandleFunc("/containers/web/stats", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("stream"); got != "0" {
			t.Errorf("stream = %q, want 0", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"container_id": "abc123", "name": "web",
			"memory_usage_bytes": uint64(1024), "memory_limit_bytes": uint64(8192),
			"memory_percent": 12.5, "cpu_percent": 3.25, "cpu_count": float64(4),
			"pids_current": uint64(7), "io_read_bytes": uint64(100),
			"io_write_bytes": uint64(200), "disk_usage_bytes": uint64(300),
		})
	})
	mux.HandleFunc("/containers/web/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Id":"abc123","Name":"/web"}`))
	})
	return httptest.NewServer(mux)
}

func TestRemoteExecPrintsOutputAndPropagatesExitCode(t *testing.T) {
	srv := remoteStub(t)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { Exec([]string{"web", "echo", "hi"}) })
	if out != "hi\n" {
		t.Fatalf("stdout = %q; want %q", out, "hi\n")
	}
	if *got != 3 {
		t.Fatalf("exit code = %d; want 3", *got)
	}
}

func TestRemoteExecInteractiveFailsLoudly(t *testing.T) {
	srv := remoteStub(t)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	errOut := captureStderr(t, func() { Exec([]string{"-i", "web", "echo", "hi"}) })
	if *got != 1 {
		t.Fatalf("exit code = %d; want 1", *got)
	}
	if !strings.Contains(errOut, "TTY") {
		t.Fatalf("stderr = %q; want TTY hint", errOut)
	}
}

func TestRemoteTopPrintsTitles(t *testing.T) {
	srv := remoteStub(t)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	out := captureStdout(t, func() { Top([]string{"web"}) })
	if !strings.Contains(out, "PID") || !strings.Contains(out, "CMD") {
		t.Fatalf("stdout = %q; want titles PID CMD", out)
	}
}

func TestRemoteStatsPrintsSnapshot(t *testing.T) {
	srv := remoteStub(t)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	out := captureStdout(t, func() { Stats([]string{"--no-stream", "web"}) })
	if !strings.Contains(out, "web") {
		t.Fatalf("stdout = %q; want snapshot for web", out)
	}
}

func TestRemoteInspectPrintsRawJSON(t *testing.T) {
	srv := remoteStub(t)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	out := captureStdout(t, func() { Inspect([]string{"web"}) })
	if strings.TrimSpace(out) != `{"Id":"abc123","Name":"/web"}` {
		t.Fatalf("stdout = %q; want raw inspect JSON", out)
	}
}

func TestRemoteEventsStreamsBothFrames(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"container\",\"actor_id\":\"aaa\",\"actor_name\":\"web\",\"status\":\"start\",\"time\":\"2026-01-02T15:04:06Z\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"container\",\"actor_id\":\"bbb\",\"actor_name\":\"db\",\"status\":\"die\",\"time\":\"2026-01-02T15:04:07Z\"}\n\n")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	out := captureStdout(t, func() { Events([]string{"--since", "2026-01-02T15:04:05Z"}) })
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %d (%q); want 2 JSON events", len(lines), out)
	}
	if !strings.Contains(lines[0], `"actor_id":"aaa"`) || !strings.Contains(lines[1], `"actor_id":"bbb"`) {
		t.Fatalf("stdout = %q; want both streamed events", out)
	}
}

func TestRemoteStartPrintsShortID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/web/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { StartCmd([]string{"web"}) })
	if *got != -1 {
		t.Fatalf("exit code = %d; want no exit", *got)
	}
	if strings.TrimSpace(out) != "web" {
		t.Fatalf("stdout = %q; want %q", out, "web")
	}
}

func TestRemoteStopAlreadyStoppedSucceeds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/idle/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNotModified)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { Stop([]string{"idle"}) })
	if *got != -1 {
		t.Fatalf("exit code = %d; want no exit", *got)
	}
	if strings.TrimSpace(out) != "idle" {
		t.Fatalf("stdout = %q; want %q", out, "idle")
	}
}

func TestRemoteRestartSucceeds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/web/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { Restart([]string{"web"}) })
	if *got != -1 {
		t.Fatalf("exit code = %d; want no exit", *got)
	}
	if strings.TrimSpace(out) != "web" {
		t.Fatalf("stdout = %q; want %q", out, "web")
	}
}

func TestRemoteStopAllStopsEach(t *testing.T) {
	var stops []string
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Id":"aaaabbbbccccdddd","Names":["/web1"]},{"Id":"eeeeffff00001111","Names":["/web2"]}]`))
	})
	mux.HandleFunc("/containers/web1/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stops = append(stops, "web1")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/containers/web2/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		stops = append(stops, "web2")
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { Stop([]string{"--all"}) })
	if *got != -1 {
		t.Fatalf("exit code = %d; want no exit", *got)
	}
	if len(stops) != 2 {
		t.Fatalf("stop calls = %d (%v); want 2", len(stops), stops)
	}
	for _, want := range []string{shortID("aaaabbbbccccdddd"), shortID("eeeeffff00001111")} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q; want short id %q", out, want)
		}
	}
}

func TestRemoteLogsFollowStreamsBothChunks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/web/logs", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("follow"); got != "1" {
			http.Error(w, "follow required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, "chunk1\n")
		fl.Flush()
		_, _ = fmt.Fprint(w, "chunk2\n")
		fl.Flush()
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	setRemote(t, srv.URL, "")
	got, restore := stubExit(t)
	defer restore()
	out := captureStdout(t, func() { Logs([]string{"-f", "web"}) })
	if *got != -1 {
		t.Fatalf("exit code = %d; want no exit", *got)
	}
	if !strings.Contains(out, "chunk1\n") || !strings.Contains(out, "chunk2\n") {
		t.Fatalf("stdout = %q; want both chunks", out)
	}
}
