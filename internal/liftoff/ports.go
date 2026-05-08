package liftoff

import (
	"fmt"
	"net"
	"time"
)

// Ports captures the five-port band assigned to one slot.
type Ports struct {
	Slot     int
	App      int
	Admin    int
	API      int
	AdminBE  int
	MCP      int
}

// PortsForSlot returns the deterministic port band for a slot.
// Slot 0 is master defaults: 3000/3001/9000/9001/9002.
// Slot N (N >= 1) is offset by N*10.
func PortsForSlot(slot int) Ports {
	return Ports{
		Slot:    slot,
		App:     3000 + slot*10,
		Admin:   3001 + slot*10,
		API:     9000 + slot*10,
		AdminBE: 9001 + slot*10,
		MCP:     9002 + slot*10,
	}
}

// All returns all five ports as a slice (handy for iteration).
func (p Ports) All() []int {
	return []int{p.App, p.Admin, p.API, p.AdminBE, p.MCP}
}

// PortsBindable returns true if every port in the slot's band can be opened
// for listen on 127.0.0.1 right now. Used at allocation time to bump past
// slots whose ports are squatted by another tool.
func PortsBindable(slot int) bool {
	for _, p := range PortsForSlot(slot).All() {
		if !portBindable(p) {
			return false
		}
	}
	return true
}

func portBindable(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// PortListening returns true if something is actively listening on
// 127.0.0.1:port. Used for service health detection (opposite of bindable).
func PortListening(port int) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
