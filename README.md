# kit

Manage isolated Liftoff feature worktrees with a Bubble Tea TUI. Inspired by
[par](https://github.com/coplane/par) — but stripped down to what actually
matters for the Liftoff dev loop: clean naming, automatic env/db/dep wiring,
graphite tracking, and a ghostty workspace per feature.

```
kit dress voice-agent     # walks you through creating a worktree
kit lineup                # show all kits on the field
kit wash                  # picker → strip and clean up a kit
kit warmup voice-agent    # launch the ghostty workspace
kit swap voice-agent      # open the worktree in your IDE
```

Classic aliases work: `new`, `ls`, `rm`, `gtab`, `open`.

## What it does

`kit dress` walks an interactive wizard, then runs (in order):

1. `git fetch origin master:master` in the master repo
2. `git worktree add ~/liftoff/<name> -b <name> master`
3. Copies `.env`, `backend/.env`, `frontend/env/.env.local`, `frontend/admin/env/.env.local`
4. (optional) `createdb liftoff_<name>` + `pg_dump liftoff | psql liftoff_<name>` + rewrites `SQLALCHEMY_DATABASE_NAME` in the worktree's `backend/.env`
5. (optional) `pip install -q -r requirements.txt -r requirements_test.txt` in `backend/`
6. (optional) `yarn install --pure-lockfile --silent` in `frontend/app` and `frontend/admin`
7. (optional) `gt track --parent master` so graphite picks it up
8. (optional) writes `~/.config/gtab/<name>.applescript` for ghostty

A leading `liftoff-` in your input is stripped automatically. So `kit dress
liftoff-voice-agent` and `kit dress voice-agent` are equivalent.

## Install

Requires Go 1.22+. From source:

```sh
git clone https://github.com/andreicstoica/kit ~/code/kit
cd ~/code/kit
make install                  # builds to ./dist/kit, copies to ~/.local/bin/kit
```

Or with `go install`:

```sh
go install github.com/andreicstoica/kit@latest
```

Make sure `~/.local/bin` (or `$(go env GOPATH)/bin`) is on `PATH`.

## Configuration

Defaults assume the canonical Liftoff layout:

| Path                                  | Purpose                |
|---------------------------------------|------------------------|
| `~/liftoff/`                          | root                   |
| `~/liftoff/liftoff-app-master/`       | master repo            |
| `~/liftoff/<name>/`                   | feature worktrees      |
| `~/.config/gtab/<name>.applescript`   | ghostty launcher files |

Override via env vars:

| Var               | Default            |
|-------------------|--------------------|
| `KIT_ROOT`        | `~/liftoff`        |
| `KIT_MASTER_DIR`  | `liftoff-app-master` |
| `KIT_GTAB_DIR`    | `~/.config/gtab`   |
| `KIT_MAIN_BRANCH` | `master`           |
| `KIT_EDITOR`      | (unset → tries `$VISUAL`, `$EDITOR`, `zed`, `cursor`, `code`) |

## Subcommands

### `kit dress` (alias `new`) — put on a fresh kit

Interactive wizard. No flags to remember. Always prompts. Walks through:
name → DB clone? → backend deps? → frontend deps? → graphite track? → gtab?
→ review → run with live progress.

### `kit lineup` (alias `ls`) — kits on the field

Static table of every active worktree: name, branch, dirty/ahead/behind status,
DB presence, gtab presence. Detects legacy `liftoff-<name>` layouts and marks
them so you know which to migrate.

### `kit wash` (alias `rm`) — strip a kit

Bubble Tea picker → confirm with toggles for DB drop and gtab removal →
runs cleanup. Worktree and branch deletion are fatal-on-fail; DB and gtab
cleanup are best-effort.

### `kit warmup <name>` (alias `gtab`) — launch ghostty

Runs `gtab <name>` if installed, otherwise falls back to `osascript`.

### `kit swap <name>` (alias `open`) — open in IDE

Opens the worktree in the first available editor: `$KIT_EDITOR`, `$VISUAL`,
`$EDITOR`, `zed`, `cursor`, `code`.

## Roadmap

- `kit ports` — assign and persist per-worktree backend/frontend ports so
  you can run multiple features at once without spinning servers up/down

## Development

```sh
make test            # go test ./...
make vet             # go vet ./...
make fmt             # go fmt ./...
make run ARGS="lineup"
```

## License

MIT
