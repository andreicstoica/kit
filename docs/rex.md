# Rex workspace backend

Rex is Kit's default workspace runtime. Rex sessions contain windows (tabs),
and windows contain blocks (panes). Saved `settings.workspace_backend` and
`KIT_WORKSPACE_BACKEND` still override the default. `kit-herdr` selects the Herdr
fallback while sharing the same config and service ownership records.

Kit discovers `rex` on `PATH` or in the standard Rex and Rex Beta app bundles.
It uses the documented CLI and Lua automation APIs, with a five-second timeout
per CLI request and autostart disabled. `kit open` creates a session if needed and adds missing tabs from the
selected Kit layout; tabs not in the layout are left alone. `kit close` closes
the mapped Rex session. Rex state is read with a single `rex do` Lua request.
Unavailable CLI and server errors are reported rather than silently falling
back to another runtime.

Session reuse/close require the saved session ID. A matching display label on
an unmapped session is treated as a collision, never adopted or deleted. Kit
layout tabs start the same Kit log-following and agent commands used by the
Herdr adapter; the initial `shell` tab is an interactive shell.

`ReadRexState` is read-only. Rex persists its own terminal and session state;
Kit stores only the Rex session ID and selected layout in its worktree config.
The persistent `master` Rex session ID is stored separately in
`settings.rex_master_session_id`, not as a synthetic worktree. `OpenRex` does
not focus or attach; callers opt into that separately with `FocusRexClient`.
Focusing updates server focus for connected clients; bringing the Rex app to
the foreground is separate UI behavior.

Useful API entry points are `RexAvailable`, `ReadRexState`,
`OpenRex(name, path, layoutName)`, `CloseRex(name, path)`, and
`FocusRexClient(sessionID)`. Rex state structs list sessions, windows, and
blocks; terminal block status uses Rex's documented process inspection API
(foreground process, child process, or last exit result).

References:

- [Rex CLI](https://www.superlogical.com/rex/docs/automate/cli.md)
- [Shell scripting](https://www.superlogical.com/rex/docs/automate/shell-scripting.md)
- [Lua scripts](https://www.superlogical.com/rex/docs/automate/lua-scripts.md)
- [Lua API](https://www.superlogical.com/rex/docs/reference/lua.md)

## Program status

Rex supports [OSC 7501](https://www.superlogical.com/rex/docs/build/program-status),
the [program-status protocol](https://mitchellh.com/writing/program-status-osc7501).
Kit reads these records through the terminal `program_status` API and shows
counts in `kit lineup` under Rex. Missing reports show `unknown`, not `idle`.
These are program records, not a count of agents; one program can report several tasks.

Use `kit lineup --agents` for a snapshot across **all** Rex sessions,
including sessions outside Kit's checkout registry, with tab, program,
state, blocking kind, and message. This is not a live-refresh view.

Rex shows completed/blocked indicators in its session picker and working/blocked
indicators in unfocused tab headers. The left tab bar keeps workspace tabs visible
across sessions. Kit does not modify Rex's native sidebar.

Agents must emit OSC 7501 for this to work. Protocol support in Rex does not
imply support in every installed agent. Kit does not scrape terminal contents
or claim a running process is necessarily working.

## Simple layout

The `simple` Rex layout creates one unlabeled tab, which Rex names after the agent's thread title, with an empty interactive shell
on the left (one-third width) and Claude Code on the right (two-thirds).
No logs tab is created. If Claude Code is not installed, the right pane falls
back to a shell with an explanation. Existing sessions are not rebuilt and
running panes are not removed. Select it with `settings.workspace_layout = "simple"`
or `--layout simple` on commands that accept a layout.

## Native command palette

The local trial includes `dev/rex-actions.lua` and its `dev/rex-action` runner.
Load the actions from Rex's `~/.config/rex/init.lua`, adjusting the checkout path:

```lua
dofile(os.getenv("HOME") .. "/code/kit/dev/rex-actions.lua")
```

Run `rex config check` and `rex config reload`. In Rex's command palette, search
for **Kit:** actions: new and delete workspace, start, stop and restart services,
sync, lineup, agent status, parked, park, resume, and operation log. Wizards
run the real terminal UI in a temporary tab. New and delete close the tab when they
exit successfully. Start, stop, restart, sync and the read-only views wait for Enter. Park and
resume act on the selected session without a tab. Rex's sidebar and Cmd+K handle navigation.
Restart uses the existing CLI behavior: resolve the workspace from the focused
directory or offer a workspace picker, and restart the currently running services.
Use `kit restart --only ...` in a shell to choose specific services.
Restart output and command errors wait for Enter before closing. No service is restarted
just by reloading the configuration. Herdr support remains available.

The action uses `rex.session.new_window` to open a temporary
native tab. The floating-layer implementation was transparent in the macOS client;
terminal theme overrides did not make it opaque. No theme overrides are applied.
The action uses
the focused terminal's reported directory where available and runs
`~/.local/bin/kit` (override with `KIT_REX_BIN` for development). Existing panes are not
closed or replaced.

Actions invoked from the palette or a key explicitly queue the native
`session.select` action for the temporary tab. Server-side focus alone does not
select it in the macOS client. The runner starts through interactive login zsh
so `.zprofile` and `.zshrc` supply the same Node/Yarn environment as a normal
terminal, then executes the Kit runner without interpolating workspace names.

Workspace selection from the Kit CLI still requires Remote Control in Rex's
Rex Server settings. The palette runs the real terminal wizard; the public API
does not expose a generic native custom-form UI. It does not bypass
confirmation, transfer agent processes, or automatically approve permissions.

Verify the runner with `python3 dev/rex-action-test.py`. The optional live smoke
test, `python3 dev/rex-palette-smoke.py`, creates and removes a disposable Rex
session with fake Kit to check Restart-tab rendering and closure on Enter.
It never restarts real services.

## Creation and service performance

Workspace creation overlaps the ordered database-copy chain with independent
backend installation, frontend links, Graphite tracking, and workspace-file
creation. Each step reports completion immediately; rollback waits for all
branches and retains the existing durable resource ownership checks.

Frontend services start while RabbitMQ isolation is prepared; Python services
still wait for broker setup. Shutdown still checks full process-tree ownership
and uses the same TERM/KILL grace periods, but exit polling queries only the
target PIDs rather than repeatedly scanning all system processes.

After Rex-backed design, Kit finishes any selected service-start flow and opens
the new simple workspace with the Claude pane focused. Existing sessions are not
rebuilt. Native app selection requires Remote Control in Rex Server settings;
failure leaves the created workspace intact and reports how to select it.

## Database copy

`kit design` copies the local `liftoff` database with `createdb --template=liftoff
--strategy=file_copy`, a file-level copy that skips SQL replay and index rebuilds.
Postgres refuses it while anything is connected to `liftoff`. Kit then creates an
empty database and falls back to `pg_dump | psql`. A failed template copy leaves
no database behind.
