//go:build linux

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Inspect(args []string) {
	showSensitive := false
	var names []string
	for _, arg := range args {
		if arg == "--sensitive" {
			showSensitive = true
			continue
		}
		names = append(names, arg)
	}
	if len(names) < 1 {
		fmt.Println("Usage: cardinal inspect [--sensitive] <container> [<container>...]")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		inspectRemote(host, names)
		return
	}

	for _, name := range names {
		c, err := container.Load(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}
		data, err := json.Marshal(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error inspecting %s: %v\n", name, err)
			continue
		}
		var view map[string]interface{}
		if err := json.Unmarshal(data, &view); err != nil {
			fmt.Fprintf(os.Stderr, "Error preparing inspection %s: %v\n", name, err)
			continue
		}
		if !showSensitive {
			redactInspection(view)
		}
		data, err = json.MarshalIndent(view, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting inspection %s: %v\n", name, err)
			continue
		}
		fmt.Println(string(data))
	}
}

// inspectRemote prints the serve inspection body as-is per container.
// Serve returns Docker-style inspect JSON, which differs from the local
// state dump; --sensitive is accepted but is a no-op remotely (the server
// schema carries no cardinal secrets).
func inspectRemote(host string, names []string) {
	c := client.NewClient(host, remoteTokenResolved())
	for _, name := range names {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		body, err := c.Inspect(ctx, name)
		cancel()
		if err != nil {
			failf("remote %s: %v", host, err)
			return
		}
		fmt.Println(body)
	}
}

func redactInspection(view map[string]interface{}) {
	delete(view, "env")
	if secrets, ok := view["secrets"].([]interface{}); ok {
		for _, item := range secrets {
			if secret, ok := item.(map[string]interface{}); ok {
				delete(secret, "data")
			}
		}
	}
	if configs, ok := view["configs"].([]interface{}); ok {
		for _, item := range configs {
			if config, ok := item.(map[string]interface{}); ok {
				delete(config, "data")
			}
		}
	}
	for key := range view {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") {
			delete(view, key)
		}
	}
}
