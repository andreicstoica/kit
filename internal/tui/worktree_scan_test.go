package tui

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorktreeScanBoundsConcurrencyAndJoins(t *testing.T) {
	worktrees := make([]liftoff.Worktree, 20)
	started := make(chan struct{}, len(worktrees))
	release := make(chan struct{})
	defer close(release)
	done := make(chan struct{})
	var active, peak, visited atomic.Int32
	go func() {
		scanWorktrees(worktrees, func(_ int, _ liftoff.Worktree) {
			n := active.Add(1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			visited.Add(1)
		})
		close(done)
	}()
	for i := 0; i < 8; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("independent probes did not overlap")
		}
	}
	select {
	case <-started:
		t.Fatal("more than eight probes started")
	case <-done:
		t.Fatal("scan returned before joining probes")
	default:
	}
	for i := 0; i < len(worktrees); i++ {
		release <- struct{}{}
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scan did not finish")
	}
	if peak.Load() > 8 || visited.Load() != int32(len(worktrees)) {
		t.Fatalf("peak=%d visited=%d", peak.Load(), visited.Load())
	}
}
