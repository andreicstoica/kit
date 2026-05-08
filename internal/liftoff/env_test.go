package liftoff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceEnvKey_Existing(t *testing.T) {
	in := "FOO=1\nSQLALCHEMY_DATABASE_NAME=liftoff\nBAR=2\n"
	out := replaceEnvKey(in, "SQLALCHEMY_DATABASE_NAME", "liftoff_voice_agent")
	want := "FOO=1\nSQLALCHEMY_DATABASE_NAME=liftoff_voice_agent\nBAR=2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestReplaceEnvKey_Append(t *testing.T) {
	in := "FOO=1\nBAR=2\n"
	out := replaceEnvKey(in, "SQLALCHEMY_DATABASE_NAME", "liftoff_x")
	if !strings.Contains(out, "SQLALCHEMY_DATABASE_NAME=liftoff_x") {
		t.Errorf("expected key appended, got %q", out)
	}
	if !strings.Contains(out, "FOO=1\n") {
		t.Errorf("expected existing keys preserved, got %q", out)
	}
}

func TestReplaceEnvKey_NoTrailingNewline(t *testing.T) {
	in := "FOO=1\nBAR=2"
	out := replaceEnvKey(in, "BAZ", "v")
	if !strings.HasSuffix(out, "BAZ=v\n") {
		t.Errorf("expected trailing newline added before append, got %q", out)
	}
}

func TestReplaceEnvKey_OtherKeysWithMatchingPrefix(t *testing.T) {
	// Make sure SQLALCHEMY_DATABASE_NAME does not match SQLALCHEMY_DATABASE_NAME_BACKUP
	in := "SQLALCHEMY_DATABASE_NAME_BACKUP=old\nSQLALCHEMY_DATABASE_NAME=keep\n"
	out := replaceEnvKey(in, "SQLALCHEMY_DATABASE_NAME", "new")
	if !strings.Contains(out, "SQLALCHEMY_DATABASE_NAME=new\n") {
		t.Errorf("did not replace exact key: %q", out)
	}
	if !strings.Contains(out, "SQLALCHEMY_DATABASE_NAME_BACKUP=old\n") {
		t.Errorf("touched the wrong key: %q", out)
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.env")
	dst := filepath.Join(dir, "out", "dst.env")
	if err := os.WriteFile(src, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q", string(got))
	}
}
