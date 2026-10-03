package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		if got := r.URL.Query().Get("tail"); got != "10" {
			t.Errorf("tail = %q, want 10", got)
		}
		_, _ = w.Write([]byte("line1\nline2\n"))
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
