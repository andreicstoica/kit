package tui

import (
	"strings"
	"testing"

	"github.com/andreicstoica/kit/internal/liftoff"
)

func TestRexStatusShowsUnmappedSessionsAndPermissionRequests(t *testing.T) {
	state := liftoff.RexState{Sessions: []liftoff.RexSession{{Label: "kit", Windows: []liftoff.RexWindow{{Label: "main", Blocks: []liftoff.RexBlock{{ProgramStatus: liftoff.RexProgramStatus{Records: []liftoff.RexProgramRecord{{EffectiveApp: "opencode", State: "blocked", Kind: "permission", Message: "Approve build"}}}}, {}}}}}}}
	out := RenderRexProgramStatus(state)
	for _, text := range []string{"kit", "main", "opencode", "blocked (permission)", "Approve build", "unknown", "no OSC 7501 report"} {
		if !strings.Contains(out, text) {
			t.Fatalf("missing %q in %s", text, out)
		}
	}
}
