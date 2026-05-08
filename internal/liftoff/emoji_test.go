package liftoff

import "testing"

func TestEmojiFor_Keyword(t *testing.T) {
	cases := []struct {
		branch string
		want   string
	}{
		{"voice-agent", "🎤"},
		{"auth-redo", "🔐"},
		{"bugfix-login", "🔧"},
		{"react-upgrade", "⚛️"},
		{"notebook-ui", "📓"},
		{"celery-rework", "⏳"},
	}
	for _, c := range cases {
		got := EmojiFor(c.branch)
		if got != c.want {
			t.Errorf("EmojiFor(%q) = %q, want %q", c.branch, got, c.want)
		}
	}
}

func TestEmojiFor_HashFallback(t *testing.T) {
	// "zzzqx" matches no keyword; should hash into pool.
	a := EmojiFor("zzzqx")
	if a == "" {
		t.Fatal("empty emoji for non-keyword branch")
	}
	// Stable across calls.
	b := EmojiFor("zzzqx")
	if a != b {
		t.Errorf("emoji not deterministic: %q vs %q", a, b)
	}
	// In the pool.
	in := false
	for _, e := range hashPool {
		if e == a {
			in = true
			break
		}
	}
	if !in {
		t.Errorf("emoji %q not in hashPool", a)
	}
}

func TestEmojiFor_Disabled(t *testing.T) {
	t.Setenv("KIT_NO_EMOJI", "1")
	if got := EmojiFor("voice-agent"); got != "" {
		t.Errorf("with KIT_NO_EMOJI, got %q, want empty", got)
	}
}
