package liftoff

import _ "embed"

// rexSimpleWindowLabel names the simple layout's only tab. It holds the shell and
// the Claude pane, so "shell" would mislabel it.
const rexSimpleWindowLabel = "AI Chat"

// Rex automation scripts run through `rex do -e`. They live as files so they
// are readable and can be loaded into `rex do` by hand while debugging.
var (
	// Atomic publication prevents a failed agent-pane creation from leaving a half-built session.
	//go:embed lua/simple_session.lua
	rexSimpleSessionLua string
	//go:embed lua/snapshot.lua
	rexSnapshotLua string
	//go:embed lua/create_session.lua
	rexCreateSessionLua string
	//go:embed lua/ensure_tabs.lua
	rexEnsureTabsLua string
)
