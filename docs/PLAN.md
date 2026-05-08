# kit: design + roadmap

Living design doc for `kit` — the CLI that manages parallel Liftoff feature
worktrees with auto port allocation and one-command service spin-up.

This doc captures **what we built**, **why we built it that way**, and
**what's intentionally deferred**. The README is the user-facing surface;
this is the engineering counterpart.

## Goals

1. Run two (or three) Liftoff features side-by-side without port conflicts.
2. Make worktree creation a one-command operation: clone + DB + deps +
   workspace + port slot, all wired correctly.
3. Service lifecycle: `play` to start everything, `pause` to stop, `log` to
   tail, `wash` to clean up.
4. Predictable port slots so users can memorize "slot 1 → :3010 / :9010".
5. Polished TUI for the interactive paths (Bubble Tea + huh forms).
6. Out-of-the-box experience for teammates — public Go install path.

## Non-goals

1. Sandboxed celery per worktree — Liftoff's backend hardcodes Redis DB 0
   and the default celery queue. Real isolation requires a small backend PR
   (~12 lines). Until then, `kit play` treats celery as a global service
   and offers kill-and-replace when switching worktrees.
2. Hot reconfiguration of running services. Restart on edit.
3. Cross-machine / remote worktrees. Local only.

## Port slot scheme

State file at `~/.config/kit/state.toml`:

```toml
schema = 1

[worktrees.voice-agent]
slot       = 1
created    = "2026-05-08T14:32:00Z"
last_used  = "2026-05-08T16:01:00Z"
```

Slot 0 is reserved for master defaults (3000/3001/9000/9001/9002). Slots
1–99 are allocated to worktrees. Each slot reserves a 10-port band:

| Service          | Formula           | Slot 1 | Slot 2 |
|------------------|-------------------|--------|--------|
| frontend/app     | `3000 + slot*10`  | 3010   | 3020   |
| frontend/admin   | `3001 + slot*10`  | 3011   | 3021   |
| backend/api      | `9000 + slot*10`  | 9010   | 9020   |
| backend/admin    | `9001 + slot*10`  | 9011   | 9021   |
| MCP server       | `9002 + slot*10`  | 9012   | 9022   |

**Allocation**: linear scan for the lowest unused slot ≥ 1. Atomic write
(`state.toml.tmp` → fsync → rename).

**Collision-aware**: after picking a slot, `net.Listen("tcp", "127.0.0.1:N")`
verifies all 5 ports in the band are bindable. If any port is in use by
something outside `kit`, bump to the next slot. Caps at slot 99.

**Why deterministic over dynamic**: a coworker's bash equivalent uses
"first free port from 9100" allocation. We chose deterministic slots so the
user can memorize URLs and they survive `pause`/`play` cycles. Collision
check covers the rare edge case where a port is squatted.

## Runtime env injection (no file mutation)

Frontend services need to know which backend to talk to. Instead of writing
ports to env files at `dress` time (which would dirty the worktree's git
status), we inject env vars when spawning each service via `exec.Cmd.Env`:

| Service       | Env vars set at launch |
|---------------|------------------------|
| frontend/app  | `VITE_APP_API_URL=http://localhost:<api>/api`, `VITE_APP_BASE_URL=http://localhost:<app>`, `VITE_APP_SHORT_BASE_URL=localhost:<app>` |
| frontend/admin | `VITE_APP_API_URL=http://localhost:<admin_be>/api`, `VITE_APP_BASE_URL=http://localhost:<admin>`, `VITE_APP_LIFTOFF_BASE_URL=http://localhost:<app>`, `VITE_APP_SHORT_BASE_URL=localhost:<admin>` |
| backend/api   | (none — port comes from CLI flag) |
| backend/admin | (none) |
| celery, beat  | (none) |

Env files are still copied from master at `dress` time as a baseline (keeps
secrets / feature flags / DB URL) but runtime ports are layered on top via
`os.Environ() + spec.Env`.

**Side benefit**: `git diff` from a worktree stays clean.

## Service runner

Per-worktree runtime dir: `~/.config/kit/run/<name>/`

- `<service>.pid` — leader PID (process group leader)
- `<service>.log` — combined stdout+stderr (truncated on each `play`)
- `<service>.cmd` — the exact command + env, for debugging

**Process model**:

- `exec.Command(...)` with `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`
- `cmd.Stdin = nil`, both Stdout and Stderr → log file
- `cmd.Start()` (no Wait); harvest PID; write PID file
- Liveness: `syscall.Kill(pid, 0)` returns ESRCH if dead
- Stop: `SIGTERM` to the process group, wait 3s, `SIGKILL` if still alive

**Service definitions** (cwd = worktree path; ports substituted from slot):

| Key       | Cwd              | Command (dev mode) |
|-----------|------------------|--------------------|
| `app`     | `frontend/app`   | `yarn dev --port <app>` |
| `admin`   | `frontend/admin` | `yarn dev --port <admin>` |
| `api`     | `backend`        | `uvicorn api.app:create_app --factory --host 127.0.0.1 --port <api> --reload` |
| `admin_be`| `backend`        | `uvicorn admin.admin_app:create_admin_app --factory --host 127.0.0.1 --port <admin_be> --reload` |
| `celery`  | `backend`        | `celery -A common.celery worker --loglevel=INFO` |
| `beat`    | `backend`        | `celery -A common.celery beat --loglevel=INFO` |
| `mcp`     | `backend`        | `uvicorn mcp_server.app:create_app --factory --host 127.0.0.1 --port <mcp> --reload` (opt-in) |

Backend commands are wrapped in `bash -lc 'source <venv>/bin/activate && exec ...'`
so the python venv is active. Venv path configurable via `KIT_PY_VENV`.

`uvicorn --reload` (not gunicorn) for dev: hot reload, single process,
~150 MB RSS each. Vite is ~80 MB. Two worktrees = ~1 GB total.

## node_modules symlinking

Adopted from coworker's `wt`: instead of `yarn install` per worktree
(2 GB + 2 minutes), symlink `frontend/app/node_modules` and
`frontend/admin/node_modules` to the master repo's. Done at `dress` time,
toggleable.

**Wrinkle**: if master's `package.json` drifts from the worktree's, the
symlink runs stale modules. `LinkResult.StaleLockfile` detects this and
warns. User can `yarn install --pure-lockfile` in the worktree to break
the symlink and create a real `node_modules`.

## Celery: kill-and-replace

Coworker's pattern: celery is treated as a single global service. `kit play
feat-B` checks for any running `celery.pid` across all worktrees; if one
exists, prompts to kill it and start feat-B's. Default `Y`.

**Caveats**:
- feat-A's frontend can still run; it just won't have a celery backing it
- Tasks enqueued to broker land in feat-B's queue and run against feat-B's
  code (existing pre-`kit` reality on master)

**Real isolation** requires the Liftoff backend PR documented in §Roadmap.

## Branch emoji

Cosmetic touch from coworker's `wt`. Each worktree gets a deterministic
emoji shown in `kit lineup`, the play picker, and the gtab tab title.

- Keyword match first: `*-fix-*` → 🔧, `*-search-*` → 🔍, `*-auth-*` → 🔐, etc.
- Hash fallback into a ~95-emoji pool for uniqueness without keyword meaning
- Pure function in `internal/liftoff/emoji.go`
- Disabled via `KIT_NO_EMOJI=1`

## Last-used tracking

`state.toml.last_used` updated on:

- `kit play <name>` (or via picker)
- `kit swap <name>` opens the IDE
- (deferred) shell hook on `cd` into a worktree

`kit lineup` sorts rows by `last_used` desc.

## Charm/bubble component map

| Component | Where it's used |
|-----------|-----------------|
| `huh` | `kit design` form (input + 4 confirms with validation) |
| `bubbles/help` + `bubbles/key` | Footer in design, play, pause, wash, tear, log |
| `bubbles/list` | Worktree picker in play, pause, wash |
| `bubbles/spinner` | Per-step spinner in run screens |
| `bubbles/progress` | Overall progress bar in design + play |
| `bubbles/stopwatch` | Elapsed-time display during runs |
| `bubbles/viewport` | Scrollable log viewer in `kit log` |
| `harmonica` | Spring physics for the penalty-kick animation in `kit design` |
| `lipgloss/table` | Lineup table (ANSI + emoji-width aware) |
| `glamour` | Cobra `--help` Long-body markdown rendering |
| `charmbracelet/log` | Top-level error output (`ERRO kit: …`) |

## Subcommand UX

| Command | Aliases | Purpose |
|---------|---------|---------|
| `kit design` | `new` | Create new worktree (huh wizard + Bubble Tea progress) |
| `kit lineup` | `ls`, `list` | List active worktrees |
| `kit play [name]` | — | Start services (picker if no name; cwd-aware) |
| `kit pause [name]` | — | Stop services |
| `kit log [name]` | — | Tail all `.log` files (scrollable) |
| `kit wash` | `rm`, `remove` | Strip a single kit (picker) |
| `kit tear` | `prune` | Bulk-wash merged/closed branches (multi-select) |
| `kit warmup <name>` | `gtab` | Launch ghostty workspace |
| `kit swap <name> [editor]` | `open` | Open worktree in editor |

Flags worth knowing:
- `kit play <name> --only api,app` — skip toggle screen
- `kit play <name> --no-celery` — skip celery worker + beat
- `kit pause <name> --only celery` — selective stop
- `kit pause --all` — kill everything across every worktree
- `KIT_EDITOR=cursor kit swap <name>` — force editor (also `kit swap <name> cursor`)

## Migration for legacy worktrees

Six worktrees in `~/liftoff/liftoff-*` were created by the old zshrc
script. Behavior:

- `kit lineup` displays them with `(legacy)` marker, `slot: —`
- `kit play <legacy>` lazily allocates a slot (writes to `state.toml`)
- Symlinking node_modules + service start work identically
- `kit warmup <legacy>` finds gtabs at the legacy filename
  (`liftoff-<name>.applescript`)
- `kit wash` works as today

## Configuration knobs (env vars)

| Var               | Default              | Purpose |
|-------------------|----------------------|---------|
| `KIT_ROOT`        | `~/liftoff`          | worktree root |
| `KIT_MASTER_DIR`  | `liftoff-app-master` | master repo subdir name |
| `KIT_GTAB_DIR`    | `~/.config/gtab`     | ghostty applescript dir |
| `KIT_STATE_DIR`   | `~/.config/kit`      | state.toml location |
| `KIT_RUN_DIR`     | `~/.config/kit/run`  | per-worktree run dirs |
| `KIT_MAIN_BRANCH` | `master`             | upstream branch |
| `KIT_PY_VENV`     | `~/.envs/py314`      | python venv for backend services |
| `KIT_EDITOR`      | (unset)              | force editor for `swap` |
| `KIT_NO_EMOJI`    | (unset)              | disable emoji prefixes |

## Roadmap

### Liftoff backend PR (unblocks per-worktree celery)

```python
# backend/common/config/settings.py
VALKEY_DB: int = 0
CELERY_TASK_DEFAULT_QUEUE: str = "default"
```

Then update:
- `backend/common/celery.py:18-26` — use `f"...:6379/{settings.VALKEY_DB}"`,
  add `app.conf.task_default_queue = settings.CELERY_TASK_DEFAULT_QUEUE`
- `backend/common/utils/fastapi_lifespan.py:43` — same `:6379/{settings.VALKEY_DB}`

After this lands, `kit serve.go` adds `-Q liftoff_<name>_default` to the
celery worker command and exports `VALKEY_DB=<slot>` per worktree. The
kill-and-replace prompt becomes a no-op — true parallel celery.

### `kit adopt <name>` — register legacy worktrees

Allocate a slot, symlink node_modules, optionally rename
`~/liftoff/liftoff-<name>` → `~/liftoff/<name>` via `git worktree move`.
Drops the `(legacy)` marker.

### `kit redress <name>` — rebuild from remote tip

Stash local changes (saved to `~/.config/kit/stash/<name>-<ts>.diff`),
hard-reset the branch to `origin/<name>`, re-run `dress` steps. For
"PR isn't too far along, just rebuild fresh" cases.

### Shell hook for last-used tracking

`chpwd` / `PROMPT_COMMAND` snippet that updates `last_used` when the user
`cd`s into a worktree. Distributed as a one-line snippet for `~/.zshrc`.

### Per-worktree port introspection

`kit ports <name>` — show what's actually listening on each expected port,
flag mismatches between expected and actual.

## Risks / known issues

- **Memory ceiling**: 2–3 simultaneous worktrees fits comfortably on a 16 GB
  MBP. More than that risks swapping.
- **Vite watcher load**: each Vite instance fully watches its tree. Document
  upper bound.
- **node_modules symlink staleness**: surfaced at `dress` time, not at `play`
  time. Long-lived worktrees can drift.
- **State.toml race**: two terminals running `kit design` simultaneously
  could clobber the slot file. File-lock is on the roadmap if it bites.
- **Process leak on crash**: `IsAlive(pid)` cleanup runs on every
  `play`/`pause`/`lineup`. Stale PID files get swept.

## Verification (smoke checklist)

1. `cd ~/code/kit && make test` — all unit tests pass
2. `kit design` → `feat-a` → `cat ~/.config/kit/state.toml` shows
   `[worktrees.feat-a] slot = 1`. Inspect frontend node_modules — is a symlink.
3. `kit play feat-a` → all services start; `lsof -i :3010 :3011 :9010 :9011`
   confirms.
4. Browser: `http://localhost:3010` and `http://localhost:3011` load.
5. `kit design feat-b` → slot 2. `kit play feat-b` → celery prompt
   "replace feat-a's celery? [Y/n]". Accept. Both pairs run side-by-side.
6. `kit lineup` shows 2 rows sorted by last-used.
7. `kit log feat-b` tails 4–6 streams with color-coded tags.
8. `kit pause feat-a` kills feat-a's services.
9. `kit wash feat-a` auto-stops + removes + frees slot.
10. After `gh pr merge feat-a`: `kit tear` lists feat-a as `PR MERGED`.
11. `kit design feat-c` → reuses slot 1.
