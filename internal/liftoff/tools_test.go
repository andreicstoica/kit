package liftoff

import "testing"

func TestBrewShellenvLine(t *testing.T) {
	got := BrewShellenvLine("/opt/homebrew/bin/brew")
	want := `eval "$(/opt/homebrew/bin/brew shellenv)"`
	if got != want {
		t.Fatalf("BrewShellenvLine = %q, want %q", got, want)
	}
}

func TestDetectBrewZeroValue(t *testing.T) {
	// We can't reliably mock the filesystem without dependency injection,
	// but we can assert the result is internally consistent:
	st := DetectBrew()
	if st.OnPath && st.BinaryAt == "" {
		t.Fatal("OnPath true but BinaryAt empty")
	}
	if !st.OnPath && st.PrefixDir != "" {
		t.Fatal("PrefixDir set without OnPath")
	}
}
