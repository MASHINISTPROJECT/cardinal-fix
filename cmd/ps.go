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

// psShowAll is set by the cobra wrapper before calling Ps.
var psShowAll bool

// Ps lists containers. With --host/-H it lists the remote's containers
// through `cardinal serve` instead of the local state.
func Ps(args []string) {
	if host := remoteHostResolved(); host != "" {
		psRemote(host)
		return
	}
	containers, err := container.List(psShowAll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if len(containers) == 0 {
		if psShowAll {
			fmt.Println("No containers found")
		} else {
			fmt.Println("No running containers")
		}
		return
	}

	container.PrintContainers(containers)
}

// psRemote lists containers on a remote serve endpoint.
func psRemote(host string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := c.ListContainers(ctx, psShowAll)
	if err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	if len(list) == 0 {
		if psShowAll {
			fmt.Println("No containers found")
		} else {
			fmt.Println("No running containers")
		}
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tIMAGE\tSTATUS\tNAME\tCMD")
	for _, c := range list {
		shortID := c.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		cmd := c.Command
		if len(cmd) > 40 {
			cmd = cmd[:40] + "..."
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			shortID, c.Image, c.Status, name, cmd)
	}
	_ = w.Flush()
}
