// Package client talks to a remote `cardinal serve` HTTP API. It powers the
// read-only remote mode of the CLI (`cardinal --host URL ps`), so Linux-only
// hosts become manageable from anywhere — including Windows and macOS boxes
// that cannot run containers themselves.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Summary is the subset of the serve /containers/json schema the CLI shows.
type Summary struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Command string   `json:"Command"`
}

// Client points at one `cardinal serve` base URL.
type Client struct {
	base  string
	token string
	hc    *http.Client
}

// NewClient builds a client. A missing scheme defaults to http; a trailing
// slash is trimmed.
func NewClient(base, token string) *Client {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base != "" && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	return &Client{base: base, token: token, hc: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s%s: %w (is `cardinal serve` running there?)", c.base, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("GET %s%s: status 403 (invalid or missing token — use --token or CARDINAL_TOKEN)", c.base, path)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GET %s%s: status %d: %s", c.base, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Ping returns nil when the remote serve answers.
func (c *Client) Ping(ctx context.Context) error {
	return c.get(ctx, "/_ping", nil)
}

// ListContainers mirrors `cardinal ps`: all=false lists running containers.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]Summary, error) {
	path := "/containers/json"
	if all {
		path += "?all=1"
	}
	var out []Summary
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Summary{}
	}
	return out, nil
}
