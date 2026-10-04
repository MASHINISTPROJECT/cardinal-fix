// Host-local port probing for `run` and `port add`: HostPortInUse reports
// whether a host port is already taken (e.g. cardinal-wings on 8080) so the
// CLI can warn instead of installing a DNAT rule that localhost clients
// will never reach. KnownService renders the hint text.
package portmap

import (
	"fmt"
	"net"
	"strings"
)

// knownServices maps host ports occupied by default by other components of
// the cardinal ecosystem. Used only to render actionable hints; the
// conflict check itself (HostPortInUse) is generic.
var knownServices = map[int]string{
	8080: "cardinal-wings (REST API default port; see /etc/cardinal-wings/config.toml)",
	2375: "cardinal serve (Docker-compatible API default port)",
	2022: "cardinal-wings SFTP (sftp_port default)",
	3000: "cardinal-panel (PANEL_PORT default)",
}

// KnownService names the ecosystem service that listens on port by default,
// or "" when the port is not a known default.
func KnownService(port int) string {
	return knownServices[port]
}

// HostPortInUse probes whether port/proto is already bound on this host by
// attempting a bind on loopback and on the wildcard address. It reports
// true when either bind fails.
//
// Both addresses are probed because a loopback-only listener (the default
// for cardinal-wings, cardinal serve and cardinal-panel) does not
// necessarily block a wildcard bind on Linux and vice versa; checking only
// one side would miss exactly the nginx-vs-wings conflict on 8080.
//
// Limitations: a listener bound to one specific non-loopback address (e.g.
// 192.168.1.5:8080) is not detected, and there is an inherent TOCTOU race
// between the probe and the later iptables DNAT setup. The result is a UX
// hint, not a lock — callers warn on true and must not treat false as a
// guarantee.
func HostPortInUse(port int, proto string) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	proto = strings.ToLower(strings.TrimSpace(proto))
	if proto == "" {
		proto = "tcp"
	}
	if proto == "udp" {
		return udpInUse(port)
	}
	return tcpInUse(port)
}

func tcpInUse(port int) bool {
	for _, addr := range []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("0.0.0.0:%d", port),
	} {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			return true
		}
		_ = l.Close()
	}
	return false
}

func udpInUse(port int) bool {
	for _, addr := range []string{
		fmt.Sprintf("127.0.0.1:%d", port),
		fmt.Sprintf("0.0.0.0:%d", port),
	} {
		c, err := net.ListenPacket("udp", addr)
		if err != nil {
			return true
		}
		_ = c.Close()
	}
	return false
}
