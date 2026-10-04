//go:build linux

package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Top(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cardinal top <container>")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		topRemote(host, args[0])
		return
	}

	c, err := container.Load(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if err := c.Top(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}

// topRemote prints the remote container's process list through
// `cardinal serve` as a tabwriter table.
func topRemote(host, id string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := c.Top(ctx, id)
	if err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, strings.Join(res.Titles, "\t"))
	for _, p := range res.Processes {
		if len(p) > 40 {
			p = p[:40] + "..."
		}
		_, _ = fmt.Fprintln(w, p)
	}
	_ = w.Flush()
}
