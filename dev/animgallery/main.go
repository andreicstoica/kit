// Command animgallery previews every kit design animation at once.
//
//	go run ./dev/animgallery
//
// Press q (or esc / ctrl-c) to quit.
package main

import (
	"fmt"
	"os"

	"github.com/andreicstoica/kit/internal/tui"
)

func main() {
	if err := tui.RunAnimGallery(); err != nil {
		fmt.Fprintln(os.Stderr, "animgallery:", err)
		os.Exit(1)
	}
}
