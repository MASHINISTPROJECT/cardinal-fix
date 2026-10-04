//go:build linux

package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cardinal/internal/client"
	"cardinal/internal/container"
)

func Events(args []string) {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	sinceStr := fs.String("since", "", "Show events created since timestamp")
	mustParse(fs, args, "events")

	var since time.Time
	if *sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, *sinceStr); err == nil {
			since = t
		} else if ts, err := time.Parse("2006-01-02 15:04:05", *sinceStr); err == nil {
			since = ts
		}
	}

	if host := remoteHostResolved(); host != "" {
		eventsRemote(host, since)
		return
	}

	fmt.Fprintf(os.Stderr, "Listening for events... (since %s)\n", since.Format(time.RFC3339))
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	ch := container.SubscribeEvents(100)
	defer container.UnsubscribeEvents(ch)

	enc := json.NewEncoder(os.Stdout)

	for {
		select {
		case evt := <-ch:
			if !since.IsZero() && evt.Time.Before(since) {
				continue
			}
			if err := enc.Encode(evt); err != nil {
				return
			}
		case <-sig:
			fmt.Fprintf(os.Stderr, "\n")
			return
		}
	}
}

// eventsRemote streams `cardinal serve` /events as JSON lines. The --since
// filter applies client-side, same as the local branch.
func eventsRemote(host string, since time.Time) {
	fmt.Fprintf(os.Stderr, "Listening for events... (remote %s)\n", host)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	c := client.NewClient(host, remoteTokenResolved())
	events, errs := c.StreamEvents(ctx)
	enc := json.NewEncoder(os.Stdout)
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errs:
			if err != nil {
				failf("remote %s: %v", host, err)
			}
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			if !since.IsZero() && evt.Time.Before(since) {
				continue
			}
			if err := enc.Encode(evt); err != nil {
				return
			}
		}
	}
}
