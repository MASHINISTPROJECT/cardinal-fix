//go:build linux

package cmd

import (
	"os"
	"strings"
)

// Remote-mode connection settings for the CLI. `--host/-H` points at a
// `cardinal serve` URL (e.g. http://10.0.0.2:2375); `--token` authenticates.
// Filled from cobra persistent flags for parsed commands and from
// extractRemoteFlags for legacy DisableFlagParsing commands. Env fallback:
// CARDINAL_REMOTE_HOST (deliberately NOT CARDINAL_HOST — that name already
// selects the serve *bind* address) and CARDINAL_TOKEN (the same secret the
// server itself reads, so one variable configures both sides).
var (
	remoteHost  string
	remoteToken string
)

// extractRemoteFlags pulls --host/-H/--token out of raw args (both `--flag
// value` and `--flag=value` forms) into the package vars and returns the
// remaining args for the legacy stdlib parser. A flag without a value is
// left in place so the legacy parser reports it as before.
func extractRemoteFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if v, ok := cutFlag(a, "--host"); ok {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			if v != "" {
				remoteHost = v
				continue
			}
		} else if v, ok := cutFlag(a, "-H"); ok {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			if v != "" {
				remoteHost = v
				continue
			}
		} else if v, ok := cutFlag(a, "--token"); ok {
			if v == "" && i+1 < len(args) {
				i++
				v = args[i]
			}
			if v != "" {
				remoteToken = v
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// cutFlag splits "--name=value" into ("value", true); a bare "--name"
// yields ("", true); anything else yields ("", false).
func cutFlag(arg, name string) (string, bool) {
	if arg == name {
		return "", true
	}
	if strings.HasPrefix(arg, name+"=") {
		return arg[len(name)+1:], true
	}
	return "", false
}

// remoteHostResolved returns the configured remote, flag first, env after.
func remoteHostResolved() string {
	if remoteHost != "" {
		return remoteHost
	}
	return os.Getenv("CARDINAL_REMOTE_HOST")
}

// remoteTokenResolved returns the bearer secret, flag first, env after.
func remoteTokenResolved() string {
	if remoteToken != "" {
		return remoteToken
	}
	return os.Getenv("CARDINAL_TOKEN")
}
