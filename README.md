# kit

Isolated Liftoff feature worktrees using Bubble Tea TUI. Automatic
per-worktree port allocation, one-command service spin-up, graphite-aware
lineup. Inspired by [par](https://github.com/coplane/par), shaped for the
Liftoff dev loop.

![kit lineup demo](vhs/lineup.gif)

```
kit                      # interactive menu
kit guide                # daily-flow tour
kit setup                # install tools, clone master, adopt worktrees
kit doctor               # read-only diagnosis
kit adopt <name>         # register an existing worktree
kit design <name>        # new feature worktree (wizard)
kit lineup               # table of kits
kit formation            # tree view: stack + setup + services
kit links                # print this worktree's URLs
kit play <name>          # start services
kit pause <name>         # stop services
kit log <name>           # tail logs (color-coded, / search, t filter)
kit diff                 # diff vs master (lumen-aware)
kit sync                 # gt sync + prune merged worktrees
kit wash                 # strip a kit
kit prune                # bulk-wash merged/closed worktrees
kit warmup [--detailed]  # open the Ghostty workspace
kit swap                 # open in IDE (or Ghostty)
```

Aliases: `new` (design), `ls`/`list` (lineup), `tree` (formation), `start`
(play), `stop` (pause), `logs` (log), `rm`/`remove`/`delete` (wash), `gtab`
(warmup), `open` (swap), `urls`/`ports` (links), `physio` (doctor),
`register` (adopt).

Commands that take a worktree name (`swap`, `warmup`, `play`, `pause`, `log`,
`wash`, `links`, `diff`, `adopt`) accept the same three shapes: pass a name,
omit to auto-pick from cwd, or get a numbered picker (1-9 quick-select)
otherwise. Master appears in every picker as 🚀 slot 0.

## Why

Two Liftoff features in parallel = killing backend servers, swapping
branches, restarting Vite, tracking DB versions. `kit` reduces that to
single commands:

- Each worktree gets a 5-port slot at design time (e.g. slot 1 →
  app:3010, admin:3011, api:9010, admin_be:9011).
- `kit play <name>` starts every service on those ports with frontend
  env vars pointing at the matching backend. `kit pause <name>` tears
  them down.
- `feat-a` and `feat-b` run side-by-side. No port conflicts.

## Requirements

**Required**

| Need | Why |
|------|-----|
| macOS | gtab uses AppleScript; symlink staleness check uses `stat -f` |
| Liftoff repo at `~/liftoff/liftoff-app-master/` | default layout. Override via `KIT_ROOT` / `KIT_MASTER_DIR`. |
| Python venv at `~/.envs/py314` | backend activates this before launch. Override via `KIT_PY_VENV`. |
| `yarn` on PATH | Vite dev servers |
| Go 1.22+ | install-time only |

**Optional — features auto-disable when missing**

| Tool | Unlocks |
|------|---------|
| Ghostty | `kit warmup` workspace — 2 tabs (shell + combined logs) or 5 with `--detailed` |
| `pg_dump` / `psql` | "Clone local DB" toggle in `kit design` |
| `gt` (graphite) | "Track in graphite" toggle, `kit sync`, gt stack in lineup |
| `gh` (GitHub CLI) | `kit prune` checks PR state |
| `zed` / `cursor` / `code` | any one suffices for `kit swap`. Override via `KIT_EDITOR`. |
| `lumen` | nicer side-by-side `kit diff` |

## Install

Requires Go. If you don't have it:

```sh
brew install go
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Then install kit and run setup:

```sh
go install github.com/andreicstoica/kit@latest
kit setup
```

Or from a clone:

```sh
git clone git@github.com:andreicstoica/kit.git ~/code/kit
cd ~/code/kit
make install                  # → ~/.local/bin/kit
kit setup
```

Make sure `~/.local/bin` (or `$(go env GOPATH)/bin`) is on `PATH`. Run
`kit completion --help` for shell tab-completion.

## First-time setup

![kit setup demo](vhs/setup.gif)

```sh
kit setup
```

Walks the toolchain (brew, gt, gh, node/yarn, python+venv, redis, rabbitmq,
postgres, Ghostty, an editor, lumen), offers to install missing pieces via
Homebrew, runs `gh auth login` if needed, clones the Liftoff master repo,
runs `yarn install` so frontend node_modules symlinks work, and bulk-adopts
any existing worktrees.

Interactive, idempotent. Re-run any time. For a read-only report, use
`kit doctor`.

## What `kit design` does

Wizard asks: name → clone DB? → symlink node_modules? → graphite track?

Then runs:

1. `git fetch origin master`
2. `git worktree add ~/liftoff/<name> -b <name> origin/master`
3. Copies `.env`, `backend/.env`, `frontend/{app,admin}/env/.env.local`
4. (opt) `createdb liftoff_<name>` + `pg_dump | psql` + rewrites `SQLALCHEMY_DATABASE_NAME`
5. `pip install` in `backend/` (always)
6. (opt) Symlinks `frontend/{app,admin}/node_modules` to master
7. (opt) `gt track --parent master`
8. Writes `~/.config/gtab/<name>.applescript`
9. Allocates a port slot in `~/.config/kit/config.toml`

Then prompts: open Ghostty (simple / detailed / skip) and start servers?
Leading `liftoff-` in your input is stripped.

## Run services with `kit play` / `kit pause`

`kit play [name]`:

1. **Picker** — if no name, pick from worktrees (sorted by slot)
2. **Service toggle** — defaults: `app frontend, admin frontend, app backend, admin backend, celery worker` (MCP off). Each row shows current running state.
3. **Celery prompt** — if another worktree owns celery, confirm kill-and-replace
4. **Adopt prompt** — if the worktree isn't in `config.toml` yet, confirm before allocating
5. **Live progress** — services start in parallel, ✓ when port responds
6. **Done** — URLs printed

```
$ kit play voice-agent
✓ voice-agent playing — slot 1
  app frontend:    http://localhost:3010
  admin frontend:  http://localhost:3011
  app backend:     http://localhost:9010
  admin backend:   http://localhost:9011
  celery worker:   pid 41234
  celery beat:     pid 41235

logs: ~/.config/kit/run/voice-agent/
```

Flags:

```sh
kit play voice-agent --only api,app
kit play voice-agent --no-celery
kit pause voice-agent
kit pause voice-agent --only celery
kit pause --all          # confirms before killing everything
```

## Port slot scheme

| Service          | Formula           |
|------------------|-------------------|
| app frontend     | `3000 + slot*10`  |
| admin frontend   | `3001 + slot*10`  |
| app backend      | `9000 + slot*10`  |
| admin backend    | `9001 + slot*10`  |
| MCP              | `9002 + slot*10`  |

Slot 0 is master (3000/3001/9000/9001/9002). `kit design` picks the lowest
free slot ≥ 1; bumps past any port already in use by something outside kit.

Frontend env vars (`VITE_APP_API_URL` etc.) are **injected at runtime** —
worktree env files stay textually identical to master.

## Celery

Liftoff hardcodes Redis DB `0` and the default celery queue, so two
worktrees can't both safely run celery against the same broker. `kit play`
treats celery as a single global service: if another worktree owns the
celery PID, it asks to kill-and-replace (default Yes).

True per-worktree celery is a ~12-line Liftoff backend PR — see
`internal/liftoff/serve.go` comments.

## Adoption

A "managed" worktree has an entry in `~/.config/kit/config.toml` with a port
slot and metadata. `kit design` creates managed worktrees automatically;
pre-existing worktrees need `kit adopt`.

```sh
kit adopt              # picker over unmanaged worktrees only
kit adopt voice-agent  # adopt by name
kit adopt -y           # no prompts
```

`kit setup` bulk-adopts during onboarding. `kit play <unmanaged>` inlines
the adopt prompt rather than allocating silently.

## Logs

`kit log [name]` opens a multi-tail viewer:

| Key      | Action                              |
|----------|-------------------------------------|
| `f`      | toggle auto-scroll                  |
| `/`      | substring search                    |
| `t`      | filter services panel               |
| `↑↓ k j` | scroll line                         |
| `pgup`/`pgdn`, `g`/`G` | scroll page / top / bottom |
| `q`, `ctrl+c` | exit                           |

Service tags are color-coded and padded. `--delete-all` truncates every
`.log` (files stay so running tails keep their FD). `--wait` opens the
viewer even when nothing is playing.

## Configuration

`~/.config/kit/config.toml` holds runtime state and durable settings:

```toml
schema = 2

[settings]
root         = "/Users/acs/liftoff"
master_dir   = "liftoff-app-master"
editor       = "zed"
liftoff_repo = "https://github.com/liftoff-inc/liftoff-app.git"

[worktrees.voice-agent]
slot      = 1
created   = 2026-05-08T14:32:00Z
last_used = 2026-05-11T16:01:00Z
branch    = "acs/voice-agent-cleanup"
path      = "/Users/acs/liftoff/voice-agent"
adopted   = false
```

`kit setup` writes `[settings]` from what it learned (clone path, first
installed editor). Hand-editable; re-running setup leaves non-empty fields
alone. Env vars still override config values for CI / power users:

| Var               | Default                 |
|-------------------|-------------------------|
| `KIT_ROOT`        | `~/liftoff`             |
| `KIT_MASTER_DIR`  | `liftoff-app-master`    |
| `KIT_GTAB_DIR`    | `~/.config/gtab`        |
| `KIT_STATE_DIR`   | `~/.config/kit`         |
| `KIT_RUN_DIR`     | `~/.config/kit/run`     |
| `KIT_MAIN_BRANCH` | `master`                |
| `KIT_PY_VENV`     | `~/.envs/py314`         |
| `KIT_EDITOR`      | (auto-detect)           |
| `KIT_NO_EMOJI`    | (unset = emoji on)      |

## Subcommands

### `kit design [name]` (alias `new`) — new kit

Wizard: name → clone DB? → symlink node_modules? → graphite track? Backend
deps + gtab + slot allocation always run. Trailing prompt picks the Ghostty
layout (simple / detailed / skip) and offers to start servers.

### `kit lineup` (alias `ls`) — list kits

Table: `NAME · SLOT · RUNNING · BRANCH · STATUS`. Branch emoji prefix.
Master at slot 0 with 🚀.

### `kit formation` (alias `tree`) — tree view

Hierarchical view: master root, worktrees as children, each expanded
into its gt stack, a `setup` sub-node (db ownership + node_modules
wiring), and running services.

### `kit play [name]` (alias `start`) — run servers

Wizard or direct (with `--only`). Parallel starts with port-aware env
injection.

### `kit pause [name]` (alias `stop`) — stop servers

Picker → confirm → kill (parallel). `--all` stops everything everywhere
(confirms first).

### `kit log [name]` (alias `logs`) — tail logs

Color-coded multi-tail. Keys above. `--delete-all` truncates. `--wait`
skips the "nothing playing" guard (used by the gtab logs tab).

### `kit diff [name]` — diff vs master

Uses [lumen](https://github.com/jnsahaj/lumen) when installed; falls back
to plain `git diff`. `--plain` forces plain.

### `kit sync` — daily refresh

`gt sync` in master, then prompt to `kit tear` whatever stayed merged.
Requires `gt`.

### `kit adopt [name]` (alias `register`) — register a worktree

Allocates slot + writes metadata for an existing on-disk worktree.

### `kit guide` — daily-flow tour

One-screen glamour-rendered walkthrough.

### `kit wash` (alias `rm`) — strip a kit

Picker → confirm with DB+gtab toggles → cleanup. Auto-stops services and
frees the slot.

### `kit prune` / `kit tear` — bulk cleanup

Scans for worktrees whose branch is merged into master or whose PR is
MERGED/CLOSED. Multi-select → washes each.

### `kit warmup [name]` (alias `gtab`) — Ghostty workspace

Default: 2 tabs — worktree root (run Claude / git / CLI here) and a `logs`
tab running `kit log --wait` (color-coded multi-service viewer).

`kit warmup --detailed` (or `-d`): 5 tabs — shell, frontend split (app +
admin), backend split (api + admin_be), celery, combined logs. For when
you want per-service panes side-by-side.

The AppleScript is rewritten on every run, so swapping layouts is free.

### `kit swap [name]` (alias `open`) — open in IDE or Ghostty

Picker over installed editors (Zed, Cursor, VS Code) plus Ghostty.
Auto-picks when exactly one editor is installed. `-e zed` skips the picker.

### `kit links` (aliases `ports`, `urls`) — print URLs

Prints the worktree's slot URLs with live/stopped indicators. Paste-friendly.

### `kit doctor` (alias `physio`) — diagnose

![kit doctor demo](vhs/doctor.gif)

Read-only check of every required + optional tool. Exits non-zero on any
failure (CI-friendly).

### `kit setup` — install missing tools

Interactive bootstrap. `--dry-run` walks the flow without changing anything.

## Log retention

`kit play` passively prunes `~/.config/kit/run/<name>/` dirs whose newest
file is older than 30 days and which own no live PID.

## Roadmap

- Liftoff backend PR for per-worktree Redis DB + celery queue isolation
- Shell hook so `cd` into a worktree updates `last_used`
- Restack-needed flag in the table view (already in `kit formation`)

## Development

```sh
make test            # go test ./...
make vet             # go vet ./...
make fmt             # go fmt ./...
make run ARGS="lineup"
make demo            # record GIFs from vhs/*.tape (brew install vhs)
```

## License

[PolyForm Internal Use 1.0.0](LICENSE) — source-available for internal
business use. Third-party commercial use requires a separate agreement.
