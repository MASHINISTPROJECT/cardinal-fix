package portmap

import (
	"net"
	"strings"
	"testing"
)

// holdLoopback binds an ephemeral loopback port with the given network and
// returns the port plus a release func. Skips when the sandbox forbids it.
func holdLoopback(t *testing.T, network string) (int, func()) {
	t.Helper()
	var (
		port    int
		release func()
	)
	switch network {
	case "udp":
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Skipf("cannot bind loopback udp: %v", err)
		}
		port = c.LocalAddr().(*net.UDPAddr).Port
		release = func() { _ = c.Close() }
	default:
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Skipf("cannot bind loopback tcp: %v", err)
		}
		port = l.Addr().(*net.TCPAddr).Port
		release = func() { _ = l.Close() }
	}
	if port == 0 {
		t.Fatal("ephemeral port is 0")
	}
	return port, release
}

func TestHostPortInUseTCPHeld(t *testing.T) {
	port, release := holdLoopback(t, "tcp")
	defer release()
	if !HostPortInUse(port, "tcp") {
		t.Fatalf("HostPortInUse(%d/tcp) = false while a loopback listener holds it", port)
	}
}

func TestHostPortInUseUDPHeld(t *testing.T) {
	port, release := holdLoopback(t, "udp")
	defer release()
	if !HostPortInUse(port, "udp") {
		t.Fatalf("HostPortInUse(%d/udp) = false while a loopback listener holds it", port)
	}
}

func TestHostPortInUseFree(t *testing.T) {
	port, release := holdLoopback(t, "tcp")
	release()
	// A just-closed listener never accepted connections, so no TIME_WAIT
	// state can block the rebind below.
	if HostPortInUse(port, "tcp") {
		t.Fatalf("HostPortInUse(%d/tcp) = true right after the holder closed it", port)
	}
}

func TestHostPortInUseEdge(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		if HostPortInUse(port, "tcp") {
			t.Errorf("HostPortInUse(%d/tcp) = true, want false for out-of-range port", port)
		}
	}
	// Empty protocol defaults to tcp.
	port, release := holdLoopback(t, "tcp")
	defer release()
	if !HostPortInUse(port, "") {
		t.Fatalf("HostPortInUse(%d/\"\") = false, want tcp default", port)
	}
}

func TestKnownService(t *testing.T) {
	for port, want := range map[int]string{
		8080: "cardinal-wings",
		2375: "cardinal serve",
		2022: "cardinal-wings SFTP",
		3000: "cardinal-panel",
	} {
		got := KnownService(port)
		if !strings.Contains(got, want) {
			t.Errorf("KnownService(%d) = %q, want substring %q", port, got, want)
		}
	}
	if got := KnownService(9999); got != "" {
		t.Errorf("KnownService(9999) = %q, want empty", got)
	}
}
