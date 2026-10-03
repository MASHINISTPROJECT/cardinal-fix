//go:build linux

package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Logs(args []string) {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "Follow log output")
	tail := fs.Int("tail", 0, "Show only last N lines")
	previous := fs.Bool("previous", false, "Show the previous run log")
	all := fs.Bool("all", false, "Show current and rotated logs")
	mustParse(fs, args, "logs")

	if fs.NArg() < 1 {
		fmt.Println("Usage: cardinal logs [-f] [--tail <n>] <container>")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		logsRemote(host, fs.Arg(0), *follow, *tail, *previous, *all)
		return
	}

	c, err := container.Load(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if err := c.LogsWithOptions(*follow, *tail, *previous, *all); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}

// logsRemote fetches logs through `cardinal serve`. The server supports
// tail but ignores follow (no streaming), and has no previous/rotated logs,
// so those flags fail loudly instead of silently returning partial data.
func logsRemote(host, id string, follow bool, tail int, previous, all bool) {
	if follow {
		failf("remote %s: live follow is not supported (serve ignores follow); use --tail N", host)
		return
	}
	if previous || all {
		failf("remote %s: --previous/--all need local log files; use --tail N", host)
		return
	}
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := c.Logs(ctx, id, tail)
	if err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	fmt.Print(out)
}
