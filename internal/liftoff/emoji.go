package liftoff

import (
	"hash/fnv"
	"os"
	"strings"
)

// keywordEmoji maps lowercased substrings to glyphs. First match wins;
// order matters when multiple keywords could match (e.g. "auth-bug" hits "bug" first).
var keywordEmoji = []struct {
	needle string
	glyph  string
}{
	{"fix", "🔧"}, {"bug", "🔧"}, {"patch", "🔧"},
	{"search", "🔍"},
	{"profile", "👤"}, {"prof", "👤"},
	{"share", "🔗"}, {"invite", "🔗"},
	{"speed", "⚡"}, {"perf", "⚡"}, {"fast", "⚡"},
	{"test", "🧪"},
	{"admin", "🔑"},
	{"auth", "🔐"}, {"login", "🔐"}, {"sso", "🔐"},
	{"email", "📬"}, {"mail", "📬"}, {"notif", "📬"},
	{"chat", "💬"}, {"message", "💬"}, {"dm", "💬"},
	{"notebook", "📓"},
	{"ui", "🎨"}, {"style", "🎨"}, {"design", "🎨"},
	{"data", "🗄️"}, {"migration", "🗄️"}, {"db", "🗄️"},
	{"api", "🌐"}, {"endpoint", "🌐"}, {"route", "🌐"},
	{"refactor", "🧹"}, {"clean", "🧹"},
	{"doc", "📝"}, {"readme", "📝"},
	{"deploy", "🚀"}, {"ci", "🚀"}, {"cd", "🚀"},
	{"network", "🌎"}, {"connect", "🌎"},
	{"gate", "🚩"}, {"flag", "🚩"}, {"toggle", "🚩"},
	{"growth", "📊"}, {"metric", "📊"},
	{"onboard", "👋"},
	{"celery", "⏳"}, {"task", "⏳"}, {"queue", "⏳"},
	{"sync", "🔄"}, {"import", "🔄"}, {"export", "🔄"},
	{"voice", "🎤"}, {"audio", "🎤"},
	{"video", "🎥"},
	{"agent", "🤖"},
	{"react", "⚛️"},
}

// hashPool is the fallback emoji set. Branch hash → index.
// Picked for visual variety + not overlapping with keyword glyphs.
var hashPool = []string{
	// fruits / food
	"🍊", "🍋", "🍉", "🍇", "🍓", "🍑", "🥝", "🥭", "🥥", "🍍",
	"🌽", "🌶️", "🍄", "🥨", "🥞", "🍩", "🧁", "🍪", "🍫", "🍬",
	// animals
	"🦊", "🐙", "🦉", "🦄", "🦋", "🐝", "🦔", "🦦", "🦩", "🐉",
	"🦒", "🦘", "🦡", "🦨", "🦥", "🐢", "🦎", "🐳", "🦭", "🦜",
	// objects / vibes
	"🌵", "🎯", "🧩", "🪁", "🎸", "🌈", "🔮", "🧲", "🎪", "🎲",
	"🪐", "🌙", "⭐", "✨", "🌟", "🪩", "🎏", "🪄", "🎨", "🛹",
	"🪕", "🥁", "🎺", "🎻", "🎤", "🎬", "🎢", "🛸", "🪂", "🎰",
	// plants / weather
	"🌻", "🌺", "🌸", "🌼", "🍀", "🌴", "🌲", "🌿",
	// random fun (🪲 dropped — ew)
	"🧃", "🧊", "🪅", "🦴", "🪞", "🧸", "🐌",
}

// MasterEmoji is the fixed glyph used for the main repo across every
// surface (lineup, tree, pickers, gtab titles).
const MasterEmoji = "🧊"

// EmojiFor returns a stable emoji for a branch name.
// Returns "" if KIT_NO_EMOJI is set in the environment.
func EmojiFor(branch string) string {
	if os.Getenv("KIT_NO_EMOJI") != "" {
		return ""
	}
	if branch == "master" {
		return MasterEmoji
	}
	low := strings.ToLower(branch)
	for _, kv := range keywordEmoji {
		if strings.Contains(low, kv.needle) {
			return kv.glyph
		}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(branch))
	return hashPool[int(h.Sum32())%len(hashPool)]
}
