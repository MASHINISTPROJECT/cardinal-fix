//go:build linux

package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Stop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	all := fs.Bool("all", false, "Stop all running containers")
	mustParse(fs, args, "stop")

	if *all {
		if host := remoteHostResolved(); host != "" {
			stopAllRemote(host)
			return
		}
		containers, err := container.List(false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			exitFunc(1)
		}
		for _, c := range containers {
			if err := c.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping %s: %v\n", shortID(c.ID), err)
				continue
			}
			fmt.Println(shortID(c.ID))
		}
		return
	}

	remaining := fs.Args()
	if len(remaining) < 1 {
		fmt.Println("Usage: cardinal stop [--all] <container>")
		exitFunc(1)
	}

	if host := remoteHostResolved(); host != "" {
		stopRemote(host, remaining[0])
		return
	}

	c, err := container.Load(remaining[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	if err := c.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exitFunc(1)
	}

	fmt.Println(shortID(c.ID))
}

func stopRemote(host, id string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.Stop(ctx, id); err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	fmt.Println(shortID(id))
}

func stopAllRemote(host string) {
	c := client.NewClient(host, remoteTokenResolved())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	list, err := c.ListContainers(ctx, false)
	if err != nil {
		failf("remote %s: %v", host, err)
		return
	}
	for _, item := range list {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		if name == "" {
			name = item.ID
		}
		if err := c.Stop(ctx, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping %s: %v\n", shortID(item.ID), err)
			continue
		}
		fmt.Println(shortID(item.ID))
	}
}
