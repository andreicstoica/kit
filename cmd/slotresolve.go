package cmd

import "github.com/andreicstoica/kit/internal/liftoff"

func resolveSlot(cfg *liftoff.Config, name string) (int, error) {
	return cfg.SlotFor(name)
}
