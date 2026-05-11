# kit

Manage isolated Liftoff feature worktrees with a Bubble Tea TUI. Inspired by
[par](https://github.com/coplane/par) — but stripped down to what actually
matters for the Liftoff dev loop: clean naming, automatic env/db/dep wiring,
graphite tracking, ghostty workspace generation, **automatic per-worktree
port allocation**, and **one-command service spin-up/down**.

```
kit setup                 # one-time: install missing tools, clone master
kit doctor                # check your setup is ready
kit design voice-agent    # walks you through creating a worktree
kit lineup                # show all kits on the field
kit play voice-agent      # start the kit's services (frontend + backend + celery)
kit pause voice-agent     # stop them
kit log voice-agent       # tail all service logs
kit wash                  # picker → strip and clean up a kit
kit prune                 # bulk-wash worktrees whose PR is merged/closed
kit warmup voice-agent    # launch the ghostty workspace
kit swap voice-agent      # open the worktree in your IDE
```

Classic aliases work: `new`, `ls`, `rm`, `gtab`, `open`.

## Why

Working on two Liftoff features at once used to mean constantly killing
backend servers, swapping branches, restarting Vite, and remembering which
DB belonged to which feature. `kit` makes parallel feature work
single-command:

- Each worktree gets a unique **port slot** (e.g. slot 1 → app:3010,
  admin:3011, api:9010, admin_be:9011) automatically at `kit design` time
- `kit play <name>` starts all dev servers on those ports, with frontend
  env vars wired to point at the matching backend
- `kit pause <name>` cleans up
- Both can run simultaneously: `kit play feat-a` then `kit play feat-b`
- A picker with branch emoji and last-used recency sort makes it obvious
  which kit you've been working on lately

## Requirements

**Required**

| Need | Why |
|------|-----|
| macOS | gtab uses AppleScript; symlink staleness check uses `stat -f` |
| Liftoff repo at `~/liftoff/liftoff-app-master/` | default layout. Override with `KIT_ROOT` and `KIT_MASTER_DIR` env vars if your tree differs. |
| Python venv at `~/.envs/py314` | backend services activate this venv before launch. Override with `KIT_PY_VENV`. |
| `yarn` on PATH | Vite dev servers (`yarn dev --port N`) |
| Go 1.22+ | only at install time if you `go install`; not needed if you grab a prebuilt binary |

**Optional — features auto-disable when the binary is missing**

| Tool | Unlocks |
|------|---------|
| Ghostty | `kit warmup` launches a 4-tab workspace per worktree |
| `pg_dump` / `psql` | "Clone local DB" toggle in `kit design` |
| `gt` (graphite) | "Track in graphite" toggle |
| `gh` (GitHub CLI) | `kit tear` checks PR state to flag merged/closed branches |
| `zed` / `cursor` / `code` | any one suffices for `kit swap`. Override with `KIT_EDITOR`. |

## Install

From source:

```sh
git clone git@github.com:andreicstoica/kit.git ~/code/kit
cd ~/code/kit
make install                  # → ~/.local/bin/kit
```

Or with `go install`:

```sh
go install github.com/andreicstoica/kit@latest
```

Make sure `~/.local/bin` (or `$(go env GOPATH)/bin`) is on `PATH`.

Run `kit completion --help` to wire shell tab-completion (zsh / bash / fish / powershell).

## First-time setup

```sh
kit setup
```

`kit setup` walks through the tools kit depends on (Homebrew, git, gh, node,
yarn, python, redis, postgres, Ghostty, an editor), offers to install
missing ones via Homebrew, prompts for `gh auth login` if you're not
authenticated, and clones the Liftoff master repo with `yarn install`
already done so frontend node_modules symlinks work.

It's interactive — nothing is changed without confirmation — and idempotent.
Run it any time you suspect something's off. For a read-only report (no
prompts), use `kit doctor`.

## What `kit design` does

`kit design` walks an interactive wizard, then runs (in order):

1. `git fetch origin master:master` in the master repo
2. `git worktree add ~/liftoff/<name> -b <name> master`
3. Copies `.env`, `backend/.env`, `frontend/env/.env.local`, `frontend/admin/env/.env.local`
4. (optional) `createdb liftoff_<name>` + `pg_dump liftoff | psql liftoff_<name>` + rewrites `SQLALCHEMY_DATABASE_NAME` in the worktree's `backend/.env`
5. (optional) `pip install -q -r requirements.txt -r requirements_test.txt` in `backend/`
6. (optional) Symlinks `frontend/app/node_modules` and `frontend/admin/node_modules` to master (saves ~2 GB and skips a 2-min `yarn install`)
7. (optional) `gt track --parent master`
8. (optional) writes `~/.config/gtab/<name>.applescript` for ghostty
9. **Allocates a port slot**, recorded in `~/.config/kit/state.toml`

A leading `liftoff-` in your input is stripped automatically. So `kit design
liftoff-voice-agent` and `kit design voice-agent` are equivalent.

## Run services with `kit play` / `kit pause`

`kit play [name]` walks a Bubble Tea wizard:

1. **Picker** — if no name given, pick from worktrees sorted by last-used
2. **Service toggle** — defaults: `app admin api admin_be celery beat` (MCP off)
3. **Celery prompt** — if another worktree's celery is running, confirm replacing it (default Yes)
4. **Live progress** — spinner per service, ✓ when port responds
5. **Done** — URLs printed, logs path noted

```
$ kit play voice-agent
✓ voice-agent playing — slot 1
  frontend/app:    http://localhost:3010
  frontend/admin:  http://localhost:3011
  backend/api:     http://localhost:9010
  backend/admin:   http://localhost:9011
  celery worker:   pid 41234
  celery beat:     pid 41235

logs: ~/.config/kit/run/voice-agent/
```

Skip the wizard with flags:

```sh
kit play voice-agent --only api,app
kit play voice-agent --no-celery
kit pause voice-agent
kit pause voice-agent --only celery
kit pause --all
```

## Port slot scheme

Each worktree's slot determines its 5-port band:

| Service          | Formula           |
|------------------|-------------------|
| frontend/app     | `3000 + slot*10`  |
| frontend/admin   | `3001 + slot*10`  |
| backend/api      | `9000 + slot*10`  |
| backend/admin    | `9001 + slot*10`  |
| MCP server       | `9002 + slot*10`  |

Slot 0 is reserved for master defaults (3000/3001/9000/9001/9002). `kit
design` picks the lowest free slot ≥ 1; if any port in that band is
occupied by something outside `kit`, it bumps to the next slot. State lives
in `~/.config/kit/state.toml`.

Frontend env vars (`VITE_APP_API_URL`, `VITE_APP_BASE_URL`, etc.) are
**injected at runtime**, not written to env files — your `frontend/env/.env.local`
stays textually identical to master.

## Celery limitations

Liftoff's backend hardcodes the Redis DB to `0` and uses the default celery
queue with no env override. Two worktrees can't both safely run celery
against the same broker.

`kit play <name>` handles this by treating celery as a single global service:
if another worktree owns the celery PID, it asks you to confirm a kill-and-
replace (default Yes). True per-worktree celery requires a small Liftoff
backend PR (~12 lines in `backend/common/`); see `internal/liftoff/serve.go`
comments for the path.

## Configuration

Defaults assume the canonical Liftoff layout:

| Path                                  | Purpose                |
|---------------------------------------|------------------------|
| `~/liftoff/`                          | root                   |
| `~/liftoff/liftoff-app-master/`       | master repo            |
| `~/liftoff/<name>/`                   | feature worktrees      |
| `~/.config/kit/state.toml`            | slot allocations       |
| `~/.config/kit/run/<name>/`           | per-service pid + log files |
| `~/.config/gtab/<name>.applescript`   | ghostty launchers      |

Override via env vars:

| Var               | Default                 |
|-------------------|-------------------------|
| `KIT_ROOT`        | `~/liftoff`             |
| `KIT_MASTER_DIR`  | `liftoff-app-master`    |
| `KIT_GTAB_DIR`    | `~/.config/gtab`        |
| `KIT_STATE_DIR`   | `~/.config/kit`         |
| `KIT_RUN_DIR`     | `~/.config/kit/run`     |
| `KIT_MAIN_BRANCH` | `master`                |
| `KIT_PY_VENV`     | `~/.envs/py314`         |
| `KIT_EDITOR`      | (tries `$VISUAL`, `$EDITOR`, `zed`, `cursor`, `code`) |
| `KIT_NO_EMOJI`    | (unset = emoji on)      |

## Subcommands

### `kit design` (alias `new`) — put on a fresh kit

Interactive wizard. Always prompts: name → DB clone? → backend deps? →
symlink node_modules? → graphite track? → gtab? → review → run with live
progress. Allocates a port slot at the end.

### `kit lineup` (alias `ls`) — kits on the field

Static table: `NAME · SLOT · BRANCH · STATUS · RUNNING · LAST USED`.
Branch emoji prefix. Sorted by last-used desc. RUNNING shows `N/6` when at
least one default service is alive, `—` otherwise. Detects legacy
`liftoff-<name>` paths and marks them; gtab files from the legacy zshrc
script are auto-detected so `kit warmup` works on old worktrees.

### `kit play [name]` — run servers

Wizard or direct (with `--only`). Starts services with port-aware env
injection.

### `kit pause [name]` — stop servers

Picker → confirm → kill in reverse start order. `--all` stops everything
across every worktree.

### `kit log [name]` — tail logs

Multi-tail of all `.log` files in `~/.config/kit/run/<name>/`. Each line
prefixed with the service name. Ctrl-C to exit.

### `kit wash` (alias `rm`) — strip a kit

Picker → confirm with DB+gtab toggles → cleanup. Now auto-stops running
services and frees the port slot.

### `kit prune` — bulk cleanup

Scans for worktrees whose branch is merged into master or whose PR is
MERGED/CLOSED (via `gh`). Multi-select picker → confirm → washes each.

### `kit warmup <name>` (alias `gtab`) — launch ghostty

Runs `gtab <name>` if installed, otherwise falls back to `osascript`. Tab
title now includes the branch emoji.

### `kit swap <name>` (alias `open`) — open in IDE

Opens the worktree in the first available editor. Bumps `last_used` so the
kit floats to the top of `lineup`.

### `kit doctor` — diagnose your setup

Read-only check of every tool kit depends on. Prints a colored report with
a fix hint for each warning/failure. Exits non-zero on any failure (so CI
can gate on it).

### `kit setup` — install missing tools

Interactive bootstrap. Same checks as `kit doctor`, but prompts to apply
each fix: `brew install`, `gh auth login`, clone the Liftoff master repo,
`yarn install`. Idempotent; re-run any time.

## Roadmap

- Liftoff backend PR for per-worktree Redis DB + celery queue isolation
- Optional shell hook so `cd` into a worktree updates `last_used`
- Web/CLI port-conflict introspection — show what's listening on each
  expected slot port

## Development

```sh
make test            # go test ./...
make vet             # go vet ./...
make fmt             # go fmt ./...
make run ARGS="lineup"
```

## License

MIT
