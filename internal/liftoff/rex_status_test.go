package liftoff

import "testing"

func TestRexStatusIncludesDetachedOperations(t *testing.T) {
	state, err := parseRexState(`{"sessions":[{"session_id":"s","detached_blocks":[{"block_id":"job","program_status":{"records":[{"state":"working","app":"kit"}]}}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Sessions[0].ProgramStatusSummary(); got != "1 working" {
		t.Fatalf("detached operation status = %q", got)
	}
}

func TestRexProgramStatusSummary(t *testing.T) {
	state, err := parseRexState(`{"sessions":[{"session_id":"s","windows":[{"blocks":[{"program_status":{"records":[{"state":"working","app":"codex"},{"state":"blocked","kind":"permission"}]}},{"program_status":{"records":{}}}]}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := state.Sessions[0].ProgramStatusSummary(); got != "1 blocked · 1 working" {
		t.Fatalf("status = %q", got)
	}
	if got := (RexSession{}).ProgramStatusSummary(); got != "unknown" {
		t.Fatalf("missing reports = %q", got)
	}
}
