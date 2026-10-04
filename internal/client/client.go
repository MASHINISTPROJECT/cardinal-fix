// Package client talks to a remote `cardinal serve` HTTP API. It powers the
// read-only remote mode of the CLI (`cardinal --host URL ps`), so Linux-only
// hosts become manageable from anywhere — including Windows and macOS boxes
// that cannot run containers themselves.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// Info is the subset of the serve /info schema the CLI shows.
type Info struct {
	Name              string `json:"Name"`
	ServerVersion     string `json:"ServerVersion"`
	Containers        int    `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersStopped int    `json:"ContainersStopped"`
	Images            int    `json:"Images"`
	NCPU              int    `json:"NCPU"`
	MemTotal          int64  `json:"MemTotal"`
	KernelVersion     string `json:"KernelVersion"`
	OperatingSystem   string `json:"OperatingSystem"`
	Architecture      string `json:"Architecture"`
	DockerRootDir     string `json:"DockerRootDir"`
}

type ExecResult struct {
	ID       string `json:"Id"`
	Output   string `json:"Output"`
	Stderr   string `json:"Stderr"`
	ExitCode int    `json:"ExitCode"`
}

type TopResult struct {
	Titles    []string `json:"Titles"`
	Processes []string `json:"Processes"`
}

type Stats struct {
	ContainerID   string  `json:"container_id"`
	Name          string  `json:"name"`
	MemoryUsage   uint64  `json:"memory_usage_bytes"`
	MemoryLimit   uint64  `json:"memory_limit_bytes"`
	MemoryPercent float64 `json:"memory_percent"`
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCount      float64 `json:"cpu_count"`
	PIDsCurrent   uint64  `json:"pids_current"`
	IOReadBytes   uint64  `json:"io_read_bytes"`
	IOWriteBytes  uint64  `json:"io_write_bytes"`
	DiskUsage     uint64  `json:"disk_usage_bytes"`
}

type Event struct {
	Type      string    `json:"type"`
	ActorID   string    `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	ImageName string    `json:"image_name"`
	ImageTag  string    `json:"image_tag"`
	Status    string    `json:"status"`
	Time      time.Time `json:"time"`
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
	resp, err := c.doReq(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// doReq performs an authenticated GET and maps HTTP errors to messages.
// The caller owns resp.Body.
func (c *Client) doReq(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s%s: %w (is `cardinal serve` running there?)", c.base, path, err)
	}
	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s%s: status 403 (invalid or missing token — use --token or CARDINAL_TOKEN)", c.base, path)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s%s: status %d: %s", c.base, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
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

// Info mirrors `cardinal info` (remote subset: host, version, counts).
func (c *Client) Info(ctx context.Context) (Info, error) {
	var out Info
	if err := c.get(ctx, "/info", &out); err != nil {
		return Info{}, err
	}
	return out, nil
}

// Logs mirrors `cardinal logs --tail N`: tail<=0 returns the whole log.
// The server ignores follow, so streaming stays a local-only feature.
func (c *Client) Logs(ctx context.Context, id string, tail int) (string, error) {
	path := "/containers/" + id + "/logs"
	if tail > 0 {
		path += "?tail=" + strconv.Itoa(tail)
	}
	resp, err := c.doReq(ctx, path)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// StreamLogs streams `cardinal logs -f` output until the server closes the
// stream or ctx is cancelled.
func (c *Client) StreamLogs(ctx context.Context, id string, tail int, w io.Writer) error {
	path := "/containers/" + id + "/logs?follow=1"
	if tail > 0 {
		path = "/containers/" + id + "/logs?tail=" + strconv.Itoa(tail) + "&follow=1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	sc := &http.Client{Timeout: 0}
	resp, err := sc.Do(req)
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
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *Client) doPost(ctx context.Context, path string, body, out interface{}) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s%s: %w (is `cardinal serve` running there?)", c.base, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("POST %s%s: status 403 (invalid or missing token — use --token or CARDINAL_TOKEN)", c.base, path)
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("POST %s%s: status %d: %s", c.base, path, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Exec(ctx context.Context, id string, cmd []string) (ExecResult, error) {
	var out ExecResult
	if err := c.doPost(ctx, "/containers/"+id+"/exec", map[string]interface{}{"Cmd": cmd}, &out); err != nil {
		return ExecResult{}, err
	}
	return out, nil
}

func (c *Client) Start(ctx context.Context, id string) error {
	return c.doPost(ctx, "/containers/"+id+"/start", nil, nil)
}

func (c *Client) Stop(ctx context.Context, id string) error {
	return c.doPost(ctx, "/containers/"+id+"/stop", nil, nil)
}

func (c *Client) Restart(ctx context.Context, id string) error {
	return c.doPost(ctx, "/containers/"+id+"/restart", nil, nil)
}

func (c *Client) Top(ctx context.Context, id string) (TopResult, error) {
	var out TopResult
	if err := c.get(ctx, "/containers/"+id+"/top", &out); err != nil {
		return TopResult{}, err
	}
	return out, nil
}

func (c *Client) Stats(ctx context.Context, id string) (Stats, error) {
	var out Stats
	if err := c.get(ctx, "/containers/"+id+"/stats?stream=0", &out); err != nil {
		return Stats{}, err
	}
	return out, nil
}

func (c *Client) Inspect(ctx context.Context, id string) (string, error) {
	resp, err := c.doReq(ctx, "/containers/"+id+"/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) StreamEvents(ctx context.Context) (<-chan Event, <-chan error) {
	events := make(chan Event)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/events", nil)
		if err != nil {
			select {
			case errs <- err:
			case <-ctx.Done():
			}
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		sc := &http.Client{Timeout: 0}
		resp, err := sc.Do(req)
		if err != nil {
			select {
			case errs <- fmt.Errorf("GET %s/events: %w (is `cardinal serve` running there?)", c.base, err):
			case <-ctx.Done():
			}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden {
			select {
			case errs <- fmt.Errorf("GET %s/events: status 403 (invalid or missing token — use --token or CARDINAL_TOKEN)", c.base):
			case <-ctx.Done():
			}
			return
		}
		if resp.StatusCode >= 400 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			select {
			case errs <- fmt.Errorf("GET %s/events: status %d: %s", c.base, resp.StatusCode, strings.TrimSpace(string(msg))):
			case <-ctx.Done():
			}
			return
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var ev Event
			if err := json.Unmarshal([]byte(payload), &ev); err != nil {
				continue
			}
			select {
			case events <- ev:
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case errs <- err:
			case <-ctx.Done():
			}
		}
	}()
	return events, errs
}
