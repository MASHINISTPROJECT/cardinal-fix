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

func StartCmd(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: cardinal start <container>")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		startRemote(host, args[0])
		return
	}

	c, err := container.Load(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	c.Status = container.Created
	c.ResetRestartGuard()
	if err := c.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting: %v\n", err)
		exitFunc(1)
	}

	fmt.Println(shortID(c.ID))
}

func startRemote(host, id string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.Start(ctx, id); err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	fmt.Println(shortID(id))
}
