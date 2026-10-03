//go:build linux

package cmd

import "testing"

func TestExtractRemoteFlags(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        []string
		wantArgs  []string
		wantHost  string
		wantToken string
	}{
		{"empty", nil, []string{}, "", ""},
		{"host separate", []string{"--host", "http://h:2375"}, []string{}, "http://h:2375", ""},
		{"host equals", []string{"--host=http://h:2375", "-a"}, []string{"-a"}, "http://h:2375", ""},
		{"shorthand", []string{"-H", "http://h:2375"}, []string{}, "http://h:2375", ""},
		{"shorthand equals", []string{"-H=http://h:2375"}, []string{}, "http://h:2375", ""},
		{"token separate", []string{"--token", "abc"}, []string{}, "", "abc"},
		{"token equals", []string{"--token=abc"}, []string{}, "", "abc"},
		{"mixed kept", []string{"--host", "http://h:2375", "--all", "web"}, []string{"--all", "web"}, "http://h:2375", ""},
		{"command flags kept", []string{"--all", "--tail", "5"}, []string{"--all", "--tail", "5"}, "", ""},
		{"similar prefix kept", []string{"--hostname", "x"}, []string{"--hostname", "x"}, "", ""},
		{"missing value kept", []string{"--host"}, []string{"--host"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldHost, oldToken := remoteHost, remoteToken
			defer func() { remoteHost, remoteToken = oldHost, oldToken }()
			remoteHost, remoteToken = "", ""
			got := extractRemoteFlags(tc.in)
			if len(got) != len(tc.wantArgs) {
				t.Fatalf("extractRemoteFlags(%q) = %q; want %q", tc.in, got, tc.wantArgs)
			}
			for i := range got {
				if got[i] != tc.wantArgs[i] {
					t.Fatalf("extractRemoteFlags(%q) = %q; want %q", tc.in, got, tc.wantArgs)
				}
			}
			if remoteHost != tc.wantHost || remoteToken != tc.wantToken {
				t.Fatalf("host/token = %q/%q; want %q/%q", remoteHost, remoteToken, tc.wantHost, tc.wantToken)
			}
		})
	}
}
