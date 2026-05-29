package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andreicstoica/kit/internal/liftoff"
	"github.com/andreicstoica/kit/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const updateCheckInterval = 24 * time.Hour

// commandsThatSkipUpdateNudge never print the update hint — the updater itself,
// shell completion, and help/version output stay clean.
var commandsThatSkipUpdateNudge = map[string]bool{
	"update":           true,
	"completion":       true,
	"__complete":       true,
	"__completeNoDesc": true,
	"help":             true,
	"man":              true,
}

type updateCheckCache struct {
	Checked time.Time `json:"checked"`
	Latest  string    `json:"latest"`
}

func updateCheckPath() string {
	return filepath.Join(filepath.Dir(liftoff.ConfigPath()), "update-check.json")
}

// maybeNudgeUpdate prints a one-line "update available" hint on stderr for
// interactive runs, refreshing the remote tag at most once per day. It never
// blocks meaningfully (tight network timeout) and never fails the command —
// the hint goes to stderr so stdout stays clean for scripts.
func maybeNudgeUpdate(cmd *cobra.Command) {
	if commandsThatSkipUpdateNudge[cmd.Name()] {
		return
	}
	// Only nag humans — keep piped/scripted/coding-tool output clean.
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return
	}
	cur := Version()
	// Don't nag local/unreleased builds — they're intentionally ahead of tags.
	if cur == "dev" || cur == "" || strings.Contains(cur, "-dirty") || strings.Contains(cur, "devel") {
		return
	}

	cache := readUpdateCache()
	if time.Since(cache.Checked) > updateCheckInterval {
		ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
		if latest, err := latestRemoteTagCtx(ctx); err == nil {
			cache = updateCheckCache{Checked: time.Now(), Latest: latest}
			writeUpdateCache(cache)
		}
		cancel()
	}

	// Only nag when the remote tag is strictly NEWER than what we're running.
	// A plain inequality test fires backwards on local/release builds that are
	// ahead of a stale cached tag (e.g. cache says v0.1.5 while we run v0.1.6),
	// printing a nonsensical "v0.1.5 available (you have v0.1.6)".
	if semverNewer(cache.Latest, baseVersion(cur)) {
		fmt.Fprintln(os.Stderr, tui.StyleWarn.Render(
			fmt.Sprintf("kit %s available (you have %s) — run `kit update`", cache.Latest, cur)))
	}
}

// semverNewer reports whether tag a is a strictly higher version than b.
// Handles any number of dotted numeric components (v0.1.6, v0.1.6.1, …) by
// comparing field-by-field, zero-padding the shorter one. Returns false when
// either side can't be parsed — better to stay silent than nag on a version
// string we don't understand.
func semverNewer(a, b string) bool {
	pa, oka := parseSemver(a)
	pb, okb := parseSemver(b)
	if !oka || !okb {
		return false
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va != vb {
			return va > vb
		}
	}
	return false
}

func parseSemver(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

func readUpdateCache() updateCheckCache {
	var c updateCheckCache
	data, err := os.ReadFile(updateCheckPath())
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

func writeUpdateCache(c updateCheckCache) {
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(updateCheckPath()), 0o755)
	_ = os.WriteFile(updateCheckPath(), data, 0o644)
}
