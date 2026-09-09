package main

import (
	"context"
	"os"

	"charm.land/fang/v2"
	"github.com/andreicstoica/kit/cmd"
)

func main() {
	cmd.RenderMarkdownLongs()
	if err := fang.Execute(
		context.Background(),
		cmd.Root(),
		fang.WithVersion(cmd.Version()),
	); err != nil {
		os.Exit(1)
	}
}
