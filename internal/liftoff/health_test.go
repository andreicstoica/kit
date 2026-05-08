package liftoff

import (
	"net"
	"testing"
	"time"
)

func TestWaitForPort_Success(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port
	if err := WaitForPort(port, 1*time.Second); err != nil {
		t.Errorf("WaitForPort(%d) = %v, want nil", port, err)
	}
}

func TestWaitForPort_Timeout(t *testing.T) {
	// Pick a port nothing's listening on.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close() // free it; nothing now listens
	start := time.Now()
	err = WaitForPort(port, 400*time.Millisecond)
	if err == nil {
		t.Errorf("expected timeout error, got nil")
	}
	elapsed := time.Since(start)
	if elapsed < 350*time.Millisecond {
		t.Errorf("returned too quickly: %v", elapsed)
	}
}
