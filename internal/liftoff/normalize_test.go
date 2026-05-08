package liftoff

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"voice-agent", "voice-agent"},
		{"liftoff-voice-agent", "voice-agent"},
		{"  liftoff-voice-agent  ", "voice-agent"},
		{"LIFTOFF-Voice-Agent", "voice-agent"},
		{"Liftoff-Foo", "foo"},
		{"foo-bar", "foo-bar"},
		{"", ""},
	}
	for _, c := range cases {
		got := Normalize(c.in)
		if got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	good := []string{"voice-agent", "abc", "a1", "a-b-c", "feature-1"}
	bad := []string{"", "Voice", "voice_agent", "-foo", "foo-", "foo--bar", "FOO", "main", "master", "feat ure"}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("Validate(%q) unexpected error: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("Validate(%q) should have errored", s)
		}
	}
}

func TestNormalizeAndValidate(t *testing.T) {
	n, err := NormalizeAndValidate("liftoff-Voice-Agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != "voice-agent" {
		t.Errorf("got %q, want voice-agent", n)
	}
	if _, err := NormalizeAndValidate("liftoff-foo_bar"); err == nil {
		t.Errorf("should reject underscore")
	}
}

func TestDBName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"voice-agent", "liftoff_voice_agent"},
		{"foo", "liftoff_foo"},
		{"a-b-c", "liftoff_a_b_c"},
	}
	for _, c := range cases {
		if got := DBName(c.in); got != c.want {
			t.Errorf("DBName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
