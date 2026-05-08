package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [name]",
	Short: "Tail all service logs for a kit",
	Long: `log opens a multi-source tail of every .log file under
~/.config/kit/run/<name>/. Each line is prefixed with its service.

If <name> is omitted, you'll get a Bubble Tea-free picker.

Press Ctrl-C to exit.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		layout := liftoff.DefaultLayout()
		var name string
		if len(args) == 1 {
			n, err := liftoff.NormalizeAndValidate(args[0])
			if err != nil {
				return err
			}
			name = n
		} else {
			st, _ := liftoff.LoadState()
			if st == nil || len(st.Worktrees) == 0 {
				return fmt.Errorf("no worktrees in state — run `kit play` first")
			}
			out, err := tui.RenderLineup(layout)
			if err == nil {
				fmt.Println(out)
			}
			fmt.Print("name to tail: ")
			r := bufio.NewReader(os.Stdin)
			line, _ := r.ReadString('\n')
			n, err := liftoff.NormalizeAndValidate(line)
			if err != nil {
				return err
			}
			name = n
		}
		dir, err := liftoff.RunDir(name)
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("no log dir for %s — start services first", name)
		}
		var logs []string
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".log" {
				logs = append(logs, filepath.Join(dir, e.Name()))
			}
		}
		if len(logs) == 0 {
			return fmt.Errorf("no logs in %s", dir)
		}
		fmt.Fprintf(os.Stderr, "tailing %d log(s) for %s — Ctrl-C to exit\n\n", len(logs), name)
		return tailMultiple(logs)
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
}

// tailMultiple opens each path, seeks to end, and streams new lines forever.
func tailMultiple(paths []string) error {
	var wg sync.WaitGroup
	for _, p := range paths {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			tailFile(path)
		}(p)
	}
	wg.Wait()
	return nil
}

func tailFile(path string) {
	tag := filepath.Base(path)
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] open: %v\n", tag, err)
		return
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] seek: %v\n", tag, err)
		return
	}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			fmt.Printf("[%s] %s", tag, line)
		}
		if err == io.EOF {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", tag, err)
			return
		}
	}
}
