package liftoff

import "testing"

func TestOrphanCandidateResources(t *testing.T) {
	got := (OrphanCandidate{HasDB: true, HasGtab: true, HasRunDir: true, HasHerdr: true}).Resources()
	want := []string{"config", "db", "gtab", "run state", "Herdr"}
	if len(got) != len(want) {
		t.Fatalf("resources = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("resources[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
