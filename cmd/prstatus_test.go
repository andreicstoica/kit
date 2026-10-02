package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestPrintRemotePRs_IncludesMissingRecordsAndUnknownState(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh statuses", true: "lookup failed"}[failure], func(t *testing.T) {
			l := newSyncMasterRepo(t)
			t.Setenv("KIT_STATE_DIR", t.TempDir())
			path := filepath.Join(filepath.Dir(l.Master), "live")
			runTestGit(t, l.Master, "worktree", "add", path, "-b", "live")
			if err := liftoff.WithConfigLock(func(c *liftoff.Config) error {
				c.Worktrees["lost"] = liftoff.WorktreeMeta{Branch: "lost", Path: path + "-lost"}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			script := `#!/bin/sh
if [ "$1" = repo ]; then
  printf '{"nameWithOwner":"test/repo"}'
else
  case "$*" in
    *head=test:live*) printf '[[{"number":42,"state":"open","draft":true,"html_url":"https://github.com/test/repo/pull/42","head":{"ref":"live","repo":{"full_name":"test/repo"}}}]]' ;;
    *head=test:lost*) printf '[[]]' ;;
    *) exit 1 ;;
  esac
fi
`
			if failure {
				script = "#!/bin/sh\necho 'authentication failed' >&2\nexit 1\n"
			}
			if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			var out bytes.Buffer
			err := printRemotePRs(&out, l)
			if (err != nil) != failure {
				t.Fatalf("command error = %v, want error=%v", err, failure)
			}
			want := []string{"live", "lost", "present", "missing"}
			if failure {
				want = append(want, "UNKNOWN")
			} else {
				want = append(want, "#42 OPEN (draft)", "NONE", "https://github.com/test/repo/pull/42")
			}
			for _, text := range want {
				if !strings.Contains(out.String(), text) {
					t.Fatalf("output missing %q:\n%s", text, out.String())
				}
			}
		})
	}
}
