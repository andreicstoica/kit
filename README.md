# kit

Isolated Liftoff feature worktrees using Bubble Tea TUI. Automatic
per-worktree port allocation, one-command service spin-up, graphite-aware
lineup. Inspired by [par](https://github.com/coplane/par), shaped for the
Liftoff dev loop.

![kit lineup demo](vhs/lineup.gif)

```
kit                       # interactive menu — pick an action
kit guide                 # daily-flow tour
kit setup                 # install tools, clone master, adopt worktrees
kit doctor                # read-only diagnosis
kit adopt <name>          # register an existing worktree
kit design voice-agent    # new feature worktree (wizard)
kit lineup                # table of kits
kit lineup --tree         # tree + gt stack + services
kit links                 # print this worktree's URLs
kit play <name>           # start services
kit pause <name>          # stop services
kit logs <name>            # tail logs (/ search, t services, --delete-all)
kit diff                  # diff vs master (lumen-aware)
kit sync                  # gt sync + prune merged worktrees
kit wash                  # strip a kit
kit prune                 # bulk-wash merged/closed worktrees
kit warmup                # open the Ghostty workspace
kit swap                  # open in IDE (or Ghostty)
```

Aliases: `new` (design), `ls`/`list` (lineup), `start` (play), `stop` (pause),
`log` (logs), `rm`/`remove`/`delete` (wash), `gtab` (warmup), `open` (swap),
`urls`/`ports` (links), `physio` (doctor), `register` (adopt), `prune` (tear).

Commands that take a worktree name (`swap`, `warmup`, `play`, `pause`, `log`,
`wash`, `links`, `diff`, `adopt`) accept the same three shapes: pass a name,
omit to auto-pick from cwd, or get a numbered picker (1-9 quick-select)
otherwise. Master appears in every picker as 🚀 slot 0.

## Why

Two Liftoff features in parallel used to mean killing backend servers,
swapping branches, restarting Vite, and tracking which version your DB was on. 
`kit` reduces all of that to single commands:

- Each worktree gets a 5-port slot (e.g. slot 1 → app:3010, admin:3011,
  api:9010, admin_be:9011) at `kit design` time - or when setting up `kit` for the first time.
- `kit play <name>` starts every dev server on those ports with frontend env
  vars pointing at the matching backend.
- `kit pause <name>` tears them down.
- `feat-a` and `feat-b` run side-by-side, no port conflicts.

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
| Ghostty | `kit warmup` 4-tab workspace with auto-tailing logs |
| `pg_dump` / `psql` | "Clone local DB" toggle in `kit design` |
| `gt` (graphite) | "Track in graphite" toggle, `kit sync`, gt stack in lineup |
| `gh` (GitHub CLI) | `kit prune` checks PR state |
| `zed` / `cursor` / `code` | any one suffices for `kit swap`. Override via `KIT_EDITOR`. |
| `lumen` | nicer side-by-side `kit diff` |

## Install

```sh
git clone git@github.com:andreicstoica/kit.git ~/code/kit
cd ~/code/kit
make install                  # → ~/.local/bin/kit
```

Or:

```sh
go install github.com/andreicstoica/kit@latest
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

`kit design [name]` walks a wizard, then runs (in order):

1. `git fetch origin master:master`
2. `git worktree add ~/liftoff/<name> -b <name> master`
3. Copies `.env`, `backend/.env`, `frontend/env/.env.local`, `frontend/admin/env/.env.local`
4. (opt) `createdb liftoff_<name>` + `pg_dump | psql` + rewrites `SQLALCHEMY_DATABASE_NAME`
5. (opt) `pip install` in `backend/`
6. (opt) Symlinks `frontend/{app,admin}/node_modules` to master
7. (opt) `gt track --parent master`
8. (opt) Writes `~/.config/gtab/<name>.applescript`
9. Allocates a port slot in `~/.config/kit/config.toml`

Leading `liftoff-` in your input is stripped.

## Run services with `kit play` / `kit pause`

`kit play [name]`:

1. **Picker** — if no name, pick from worktrees (sorted by slot)
2. **Service toggle** — defaults: `app_front admin_front app_back admin_back celery` (MCP off). Each row shows current running state.
3. **Celery prompt** — if another worktree owns celery, confirm kill-and-replace
4. **Adopt prompt** — if the worktree isn't in `config.toml` yet, confirm before allocating
5. **Live progress** — services start in parallel, ✓ when port responds
6. **Done** — URLs printed

```
$ kit play voice-agent
✓ voice-agent playing — slot 1
  app_front:   http://localhost:3010
  admin_front: http://localhost:3011
  app_back:    http://localhost:9010
  admin_back:  http://localhost:9011
  celery:      pid 41234

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
| app_front        | `3000 + slot*10`  |
| admin_front      | `3001 + slot*10`  |
| app_back         | `9000 + slot*10`  |
| admin_back       | `9001 + slot*10`  |
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

- `f` follow / `↑↓ k j` scroll / `pgup pgdn g G`
- `/` substring search
- `t` services panel — toggle which streams show
- `q` / `ctrl+c` exit

Service tags are color-coded and padded. `--delete-all` truncates every
`.log` in the run dir (with confirm); files stay so running tails keep
their FD.

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

### `kit design` (alias `new`) — new kit

Interactive wizard: name → DB clone? → backend deps? → symlink? → graphite? →
gtab? → review → run. Allocates a port slot at the end.

### `kit lineup` (alias `ls`) — list kits

Table: `NAME · SLOT · RUNNING · BRANCH · STATUS`. Branch emoji prefix.
Master at slot 0 with 🚀.

`--tree` swaps to a hierarchical view: master root, worktrees as children,
gt stack inline, running services as a `services` sub-node.

### `kit play [name]` (alias `start`) — run servers

Wizard or direct (with `--only`). Parallel starts with port-aware env
injection.

### `kit pause [name]` (alias `stop`) — stop servers

Picker → confirm → kill (parallel). `--all` stops everything everywhere
(confirms first).

### `kit log [name]` (alias `logs`) — tail logs

Color-coded multi-tail. Keys above. `--delete-all` truncates.

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

4 tabs (root, frontend split, backend split, celery) with each pane already
`tail -F`ing the matching service log. Auto-writes the template if missing.

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
- Restack-needed flag in the table view (already in `--tree`)

## Development

```sh
make test            # go test ./...
make vet             # go vet ./...
make fmt             # go fmt ./...
make run ARGS="lineup"
make demo            # record GIFs from vhs/*.tape (brew install vhs)
```

## License

MIT
