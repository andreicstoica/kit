package liftoff

import (
	"os"
	"path/filepath"
)

// This file centralizes "where does PATH wiring go" so setup and doctor agree.
// kit installs to ~/.local/bin (make install) or GOPATH/bin (go install), and
// brew lives under /opt/homebrew or /usr/local — none guaranteed on PATH for a
// fresh teammate. The login shell decides which rc file actually loads, so we
// detect it instead of hardcoding ~/.zshrc (which silently fails for bash
// users, whose login shell reads ~/.bash_profile).

// ShellName returns the basename of the user's login shell ($SHELL), e.g.
// "zsh" or "bash". Falls back to "zsh" (the macOS default) when $SHELL is
// unset or unrecognized.
func ShellName() string {
	switch base := filepath.Base(os.Getenv("SHELL")); base {
	case "bash", "zsh":
		return base
	default:
		return "zsh"
	}
}

// ShellProfilePath returns the rc file that the user's login shell sources on
// startup — the right place to append PATH wiring.
//
//   - zsh  → $ZDOTDIR/.zshrc (or ~/.zshrc; Terminal.app runs zsh login+interactive, so .zshrc loads)
//   - bash → ~/.bash_profile (Terminal.app runs bash as a *login* shell, which
//     reads .bash_profile, NOT .bashrc — the usual footgun)
//
// Unknown shells fall back to the zsh path.
func ShellProfilePath() string {
	home, _ := os.UserHomeDir()
	if ShellName() == "bash" {
		return filepath.Join(home, ".bash_profile")
	}
	// zsh honors $ZDOTDIR as the home for its rc files when set.
	dir := home
	if z := os.Getenv("ZDOTDIR"); z != "" {
		dir = z
	}
	return filepath.Join(dir, ".zshrc")
}

// ResolvedExecutable returns the absolute, symlink-resolved path of the
// running kit binary. On EvalSymlinks failure it returns the raw os.Executable
// path (best effort); it only errors when os.Executable itself fails.
func ResolvedExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

// KitBinDir returns the directory holding the running kit binary (symlinks
// resolved, so a ~/.local/bin/kit symlink reports the real dir). On failure it
// falls back to ~/.local/bin, the `make install` target.
func KitBinDir() string {
	if exe, err := ResolvedExecutable(); err == nil {
		return filepath.Dir(exe)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// DirOnPath reports whether dir is one of the entries in $PATH (compared after
// cleaning, so trailing slashes don't cause false negatives).
func DirOnPath(dir string) bool {
	want := filepath.Clean(dir)
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(p) == want {
			return true
		}
	}
	return false
}

// PathExportLine returns a POSIX `export PATH=...` line prepending dir. Valid
// in both bash and zsh, so the same line works regardless of detected shell.
func PathExportLine(dir string) string {
	return `export PATH="` + dir + `:$PATH"`
}
