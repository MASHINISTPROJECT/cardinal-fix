package client

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	check := func(w http.ResponseWriter, r *http.Request) bool {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return false
		}
		return true
	}
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode([]Summary{
			{ID: "abc123def456", Names: []string{"/web"}, Image: "nginx:alpine", State: "running", Status: "running", Command: "nginx -g daemon off;"},
		})
	})
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(Info{
			Name: "srv-01", ServerVersion: "2.1.7-cardinal",
			Containers: 3, ContainersRunning: 2, ContainersStopped: 1,
			Images: 5, NCPU: 4, MemTotal: 8 << 30,
			KernelVersion: "6.8.0", OperatingSystem: "Ubuntu 24.04",
			Architecture: "x86_64", DockerRootDir: "/root/.cardinal",
		})
	})
	mux.HandleFunc("/containers/web/logs", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.URL.Query().Get("follow") == "1" {
			_, _ = w.Write([]byte("chunk1\n"))
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			time.Sleep(100 * time.Millisecond)
			_, _ = w.Write([]byte("chunk2\n"))
			return
		}
		if got := r.URL.Query().Get("tail"); got != "10" {
			t.Errorf("tail = %q, want 10", got)
		}
		_, _ = w.Write([]byte("line1\nline2\n"))
	})
	mux.HandleFunc("/containers/web/exec", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Cmd []string `json:"Cmd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if len(req.Cmd) != 2 || req.Cmd[0] != "echo" || req.Cmd[1] != "hi" {
			t.Errorf("Cmd = %q, want [echo hi]", req.Cmd)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Id": "abc123_exec", "Output": "hi\n", "Stderr": "", "ExitCode": 0,
		})
	})
	mux.HandleFunc("/containers/web/top", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"Titles":    []string{"PID", "CMD"},
			"Processes": []string{"1 init", "2 sh"},
		})
	})
	mux.HandleFunc("/containers/web/stats", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
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
		if !check(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"Id":"abc123","Name":"/web"}`))
	})
	mux.HandleFunc("/containers/web/start", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/containers/web/stop", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/containers/web/restart", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/containers/idle/stop", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNotModified)
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"container\",\"actor_id\":\"a1\",\"actor_name\":\"web\",\"image_name\":\"nginx\",\"image_tag\":\"alpine\",\"status\":\"start\",\"time\":\"2026-10-04T00:00:00Z\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"container\",\"actor_id\":\"a2\",\"actor_name\":\"db\",\"image_name\":\"redis\",\"image_tag\":\"7\",\"status\":\"stop\",\"time\":\"2026-10-04T00:00:01Z\"}\n\n"))
	})
	return httptest.NewServer(mux)
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestPing(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	if err := NewClient(srv.URL, "").Ping(ctx(t)); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPingBadHost(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "")
	c.hc.Timeout = 200 * time.Millisecond
	if err := c.Ping(ctx(t)); err == nil {
		t.Fatal("Ping(dead port) = nil, want error")
	}
}

func TestAuthForwarded(t *testing.T) {
	srv := testServer(t, "secret")
	defer srv.Close()
	if err := NewClient(srv.URL, "secret").Ping(ctx(t)); err != nil {
		t.Fatalf("Ping with token: %v", err)
	}
	if err := NewClient(srv.URL, "wrong").Ping(ctx(t)); err == nil {
		t.Fatal("Ping with wrong token = nil, want 403 error")
	}
}

func TestListContainers(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	list, err := NewClient(srv.URL, "").ListContainers(ctx(t), true)
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(list) != 1 || list[0].Image != "nginx:alpine" || len(list[0].Names) != 1 {
		t.Fatalf("list = %+v", list)
	}
}

func TestBaseNormalization(t *testing.T) {
	c := NewClient("example.com:2375", "")
	if c.base != "http://example.com:2375" {
		t.Fatalf("base = %q", c.base)
	}
	c = NewClient("https://example.com/", "")
	if c.base != "https://example.com" {
		t.Fatalf("base = %q", c.base)
	}
}

func TestInfo(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	info, err := NewClient(srv.URL, "").Info(ctx(t))
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "srv-01" || info.ContainersRunning != 2 || info.Images != 5 {
		t.Fatalf("info = %+v", info)
	}
}

func TestLogs(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	out, err := NewClient(srv.URL, "").Logs(ctx(t), "web", 10)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	if out != "line1\nline2\n" {
		t.Fatalf("logs = %q", out)
	}
}

func TestExec(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	res, err := NewClient(srv.URL, "").Exec(ctx(t), "web", []string{"echo", "hi"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ID != "abc123_exec" || res.Output != "hi\n" || res.Stderr != "" || res.ExitCode != 0 {
		t.Fatalf("exec = %+v", res)
	}
}

func TestTop(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	res, err := NewClient(srv.URL, "").Top(ctx(t), "web")
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(res.Titles) != 2 || res.Titles[0] != "PID" || res.Titles[1] != "CMD" {
		t.Fatalf("titles = %+v", res.Titles)
	}
	if len(res.Processes) != 2 || res.Processes[0] != "1 init" || res.Processes[1] != "2 sh" {
		t.Fatalf("processes = %+v", res.Processes)
	}
}

func TestStats(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	res, err := NewClient(srv.URL, "").Stats(ctx(t), "web")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if res.ContainerID != "abc123" || res.Name != "web" || res.MemoryUsage != 1024 || res.MemoryLimit != 8192 {
		t.Fatalf("stats = %+v", res)
	}
	if res.MemoryPercent != 12.5 || res.CPUPercent != 3.25 || res.CPUCount != 4 || res.PIDsCurrent != 7 {
		t.Fatalf("stats = %+v", res)
	}
	if res.IOReadBytes != 100 || res.IOWriteBytes != 200 || res.DiskUsage != 300 {
		t.Fatalf("stats = %+v", res)
	}
}

func TestInspect(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	out, err := NewClient(srv.URL, "").Inspect(ctx(t), "web")
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if out != `{"Id":"abc123","Name":"/web"}` {
		t.Fatalf("inspect = %q", out)
	}
}

func TestStreamEvents(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, errs := NewClient(srv.URL, "").StreamEvents(c)
	var got []Event
	timeout := time.After(10 * time.Second)
	for len(got) < 2 {
		select {
		case ev, ok := <-events:
			if !ok {
				break
			}
			got = append(got, ev)
		case err := <-errs:
			if err != nil {
				t.Fatalf("StreamEvents err: %v", err)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for events, got %d", len(got))
		}
	}
	if got[0].ActorID != "a1" || got[0].ActorName != "web" || got[0].Status != "start" {
		t.Fatalf("event[0] = %+v", got[0])
	}
	if got[1].ActorID != "a2" || got[1].ActorName != "db" || got[1].Status != "stop" {
		t.Fatalf("event[1] = %+v", got[1])
	}
	if got[0].ImageName != "nginx" || got[0].ImageTag != "alpine" || got[0].Type != "container" {
		t.Fatalf("event[0] = %+v", got[0])
	}
}

func TestStartStopRestart(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	c := NewClient(srv.URL, "")
	if err := c.Start(ctx(t), "web"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Stop(ctx(t), "web"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := c.Restart(ctx(t), "web"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if err := c.Stop(ctx(t), "idle"); err != nil {
		t.Fatalf("Stop idle (304 no-op): %v", err)
	}
}

func TestStreamLogs(t *testing.T) {
	srv := testServer(t, "")
	defer srv.Close()
	var buf bytes.Buffer
	if err := NewClient(srv.URL, "").StreamLogs(ctx(t), "web", 10, &buf); err != nil {
		t.Fatalf("StreamLogs: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "chunk1\n") || !strings.Contains(out, "chunk2\n") {
		t.Fatalf("stream = %q, want both chunks", out)
	}
}
