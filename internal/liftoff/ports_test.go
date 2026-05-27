package liftoff

import (
	"net"
	"testing"
)

func TestPortsForSlot(t *testing.T) {
	cases := []struct {
		slot int
		want Ports
	}{
		{0, Ports{Slot: 0, App: 3000, Admin: 3001, API: 9000, AdminBE: 9001, MCP: 9002}},
		{1, Ports{Slot: 1, App: 3010, Admin: 3011, API: 9010, AdminBE: 9011, MCP: 9012}},
		{5, Ports{Slot: 5, App: 3050, Admin: 3051, API: 9050, AdminBE: 9051, MCP: 9052}},
		{99, Ports{Slot: 99, App: 3990, Admin: 3991, API: 9990, AdminBE: 9991, MCP: 9992}},
	}
	for _, c := range cases {
		got := PortsForSlot(c.slot)
		if got != c.want {
			t.Errorf("PortsForSlot(%d) = %+v, want %+v", c.slot, got, c.want)
		}
	}
}

func TestPorts_All(t *testing.T) {
	all := PortsForSlot(1).All()
	if len(all) != 5 {
		t.Errorf("All len = %d, want 5", len(all))
	}
}

func TestPortBindable_FreePort(t *testing.T) {
	if !portBindable(0) { // 0 = OS-assigned, always bindable
		t.Errorf("port 0 should be bindable")
	}
}

func TestPortBindable_SquattedPort(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if portBindable(port) {
		t.Errorf("port %d should be in use", port)
	}
}

func TestPortListening(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if !PortListening(port) {
		t.Errorf("port %d should report listening", port)
	}
}

// listenIPv6 binds [::1]:0 and returns the listener + port, skipping the test
// if the host has no IPv6 loopback.
func listenIPv6(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	}
	return l, l.Addr().(*net.TCPAddr).Port
}

// TestPortListening_IPv6Only is the regression for the kit-play hang: Vite
// binds [::1] only, so an IPv4-only check reported the server dead and kit
// spawned a duplicate. PortListening must see the IPv6 listener.
func TestPortListening_IPv6Only(t *testing.T) {
	l, port := listenIPv6(t)
	defer l.Close()
	if !PortListening(port) {
		t.Errorf("port %d listening on [::1] should report listening", port)
	}
}

// TestPortBindable_IPv6Squatted: a port free on IPv4 but taken on IPv6 must
// read as NOT bindable, or slot allocation would hand out an occupied band.
func TestPortBindable_IPv6Squatted(t *testing.T) {
	l, port := listenIPv6(t)
	defer l.Close()
	if portBindable(port) {
		t.Errorf("port %d squatted on [::1] should not be bindable", port)
	}
}
