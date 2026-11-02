package tui

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"sync"
)

// Bound subprocess pressure when a machine has many worktrees.
func scanWorktrees(worktrees []liftoff.Worktree, visit func(int, liftoff.Worktree)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for i, wt := range worktrees {
		slots <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			visit(i, wt)
		}()
	}
	wg.Wait()
}
