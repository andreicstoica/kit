package liftoff

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// CopyEnvFiles copies each file in EnvFiles from src into dst.
// Missing source files are skipped (returned in skipped slice).
// Existing destination files are overwritten only if force is true; otherwise skipped.
func (l Layout) CopyEnvFiles(srcRepo, dstRepo string, force bool, onLine LineFn) (copied, skipped []string, err error) {
	for _, rel := range EnvFiles {
		src := filepath.Join(srcRepo, rel)
		dst := filepath.Join(dstRepo, rel)
		if _, e := os.Stat(src); errors.Is(e, os.ErrNotExist) {
			skipped = append(skipped, rel+" (no source)")
			if onLine != nil {
				onLine("skip " + rel + " — no source in master")
			}
			continue
		}
		if _, e := os.Stat(dst); e == nil && !force {
			skipped = append(skipped, rel+" (exists)")
			if onLine != nil {
				onLine("skip " + rel + " — already exists")
			}
			continue
		}
		if err = copyFile(src, dst); err != nil {
			return copied, skipped, err
		}
		copied = append(copied, rel)
		if onLine != nil {
			onLine("copy " + rel)
		}
	}
	return copied, skipped, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// UpdateBackendDBName rewrites SQLALCHEMY_DATABASE_NAME=... in backend/.env.
// Used when --db cloned a fresh DB and the worktree should point at it.
func (l Layout) UpdateBackendDBName(worktree, dbName string) error {
	path := filepath.Join(worktree, "backend", ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	out := replaceEnvKey(string(data), "SQLALCHEMY_DATABASE_NAME", dbName)
	return os.WriteFile(path, []byte(out), 0o600)
}

// replaceEnvKey replaces the value of `key=...` lines, leaving everything else intact.
// If key is absent, it appends `key=value` at the end.
func replaceEnvKey(content, key, value string) string {
	lines := splitLinesKeepEnding(content)
	found := false
	for i, ln := range lines {
		trim := trimEnding(ln)
		if hasEnvKey(trim, key) {
			lines[i] = key + "=" + value + lineEnding(ln)
			found = true
		}
	}
	if !found {
		sep := ""
		if len(content) > 0 && content[len(content)-1] != '\n' {
			sep = "\n"
		}
		return content + sep + key + "=" + value + "\n"
	}
	out := ""
	for _, ln := range lines {
		out += ln
	}
	return out
}

func hasEnvKey(line, key string) bool {
	if len(line) <= len(key) {
		return false
	}
	if line[:len(key)] != key {
		return false
	}
	return line[len(key)] == '='
}

func splitLinesKeepEnding(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trimEnding(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}

func lineEnding(s string) string {
	if len(s) >= 2 && s[len(s)-2] == '\r' && s[len(s)-1] == '\n' {
		return "\r\n"
	}
	if len(s) >= 1 && s[len(s)-1] == '\n' {
		return "\n"
	}
	return ""
}
