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
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		if !check(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode([]Summary{
			{ID: "abc123def456", Names: []string{"/web"}, Image: "nginx:alpine", State: "running", Status: "running", Command: "nginx -g daemon off;"},
		})
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
