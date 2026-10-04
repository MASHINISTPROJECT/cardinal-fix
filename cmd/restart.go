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

func Restart(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cardinal restart <container>")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		restartRemote(host, args[0])
		return
	}

	c, err := container.Load(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if c.Status == container.Running {
		if err := c.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping: %v\n", err)
			exitFunc(1)
		}
	}

	c.ResetRestartGuard()
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting: %v\n", err)
		exitFunc(1)
	}

	fmt.Println(shortID(c.ID))
}

func restartRemote(host, id string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.Restart(ctx, id); err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	fmt.Println(shortID(id))
}
