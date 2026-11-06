package tui

import (
	"github.com/andreicstoica/kit/internal/liftoff"
	"sync"
)

const scanWorkers = 16

func scanWorktrees(worktrees []liftoff.Worktree, visit func(int, liftoff.Worktree)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, scanWorkers)
	for i, wt := range worktrees {
		i, wt := i, wt
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
