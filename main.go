package main

import (
	"context"
	"os"

	"github.com/andreicstoica/kit/cmd"
	"github.com/charmbracelet/fang"
)

func main() {
	if err := fang.Execute(
		context.Background(),
		cmd.Root(),
		fang.WithVersion(cmd.Version()),
	); err != nil {
		os.Exit(1)
	}
}
