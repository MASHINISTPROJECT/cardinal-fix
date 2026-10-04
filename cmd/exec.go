//go:build linux

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Exec(args []string) {
	// Normalize combined shorthands like -it / -ti before flag parsing
	// stdlib flag does not handle combined bools like pflag does.
	normalized := make([]string, 0, len(args)*2)
	for _, a := range args {
		if a == "-it" || a == "-ti" {
			normalized = append(normalized, "-i", "-t")
		} else {
			normalized = append(normalized, a)
		}
	}
	args = normalized

	// Manually extract -i and -t flags from ANY position (stdlib flag.Parse
	// stops at the first non-flag arg, so "exec <id> -i /bin/sh" would miss
	// the -i flag and leak it into the nsenter command).
	var interactive, tty bool
	var remaining []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-i" {
			interactive = true
			continue
		}
		if args[i] == "-t" {
			tty = true
			continue
		}
		remaining = append(remaining, args[i])
	}

	if len(remaining) < 2 {
		fmt.Println("Usage: cardinal exec [-i] [-t] <container> <cmd> [args...]")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		execRemote(host, remaining[0], remaining[1:], interactive, tty)
		return
	}

	c, err := container.Load(remaining[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if c.Status != container.Running {
		fmt.Fprintf(os.Stderr, "Container %s is not running\n", remaining[0])
		exitFunc(1)
	}

	if err := c.ExecOpts(remaining[1:], interactive, tty); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}
}

// execRemote runs a non-interactive command through `cardinal serve`.
// Interactive/TTY sessions need a PTY and fail loudly (as logsRemote does
// for -f); the server 400s them as well, this check saves a round trip.
func execRemote(host, id string, cmd []string, interactive, tty bool) {
	if interactive || tty {
		failf("remote %s: interactive exec (-i/-t) needs a TTY; use local `cardinal exec` or the wings terminal", host)
		return
	}
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	res, err := c.Exec(ctx, id, cmd)
	if err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	fmt.Print(res.Output)
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	if res.ExitCode != 0 {
		exitFunc(res.ExitCode)
	}
}
