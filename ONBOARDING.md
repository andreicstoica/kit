# Onboarding: get set up with `kit`

You are helping a new Liftoff employee — engineer, designer building
prototypes, anyone working in the app — install **kit** and create their first
feature worktree. `kit` runs many Liftoff features at once — each in its own
git worktree with its own port slot and full service stack — so they can work
on several branches side-by-side without killing backends or fighting port
conflicts.

Walk them through the steps below **one at a time**. Run the read-only checks
yourself; for anything that installs software, mutates their machine, or needs
their credentials, show the command and let them run it (or confirm first).
Adapt to what you find — skip steps already satisfied.

## 0. Prerequisites — check before installing

- **macOS only.** kit uses AppleScript + `stat -f`. If they're on Linux, stop
  and tell them kit won't run.
- **GitHub access** to both repos: `liftoff-inc/liftoff-app` (the app) and
  `andreicstoica/kit` (the tool). Confirm `gh auth status` is authenticated;
  if not, they run `gh auth login`.
- **Homebrew** present (`brew --version`). If missing, point them at
  <https://brew.sh> — kit's setup will also print the install command.

## 1. Install Go (skip if `go version` works)

```sh
brew install go
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Confirm `~/.local/bin` and `$(go env GOPATH)/bin` are on `PATH` — kit installs
there.

## 2. Install kit

```sh
go install github.com/andreicstoica/kit@latest
```

Verify with `kit --version`. (Tab-completion: `kit completion --help`.)

## 3. Run `kit setup` — the bootstrap

```sh
kit setup
```

This is interactive and idempotent. It walks the whole toolchain (brew, gt,
gh, node/yarn, python+venv, redis, rabbitmq, postgres, Ghostty, an editor,
lumen), offers to install missing pieces via Homebrew, runs `gh auth login` if
needed, **clones the Liftoff master repo to `~/liftoff/liftoff-app-master`**,
runs `yarn install` so worktree node_modules symlinks work, and adopts any
existing worktrees. Let them answer the prompts; `--dry-run` previews without
changing anything.

## 4. Verify with `kit doctor`

```sh
kit doctor
```

Read-only. Every line should be `✓`. If anything is a ✗ or warning, re-run
`kit setup` (it only fixes what's broken) or address that specific tool.

## 5. First worktree

```sh
kit design my-first-kit
```

The wizard asks: name → clone DB? → symlink node_modules? → graphite track?
Then offers to open the editor and start servers. A worktree lands at
`~/liftoff/my-first-kit` on branch `my-first-kit`, with its own port slot.

```sh
kit play my-first-kit      # start the full service stack
kit links my-first-kit     # print the URLs
kit pause my-first-kit     # stop it
kit wash my-first-kit      # tear it down when done
```

## Everyday commands (share once they're set up)

| Command | What it does |
|---|---|
| `kit design <name>` | new feature worktree (wizard) |
| `kit lineup` / `--tree` | list kits (table / stack tree) |
| `kit play` / `pause` / `restart` | start / stop / bounce services |
| `kit log <name>` | tail color-coded service logs |
| `kit swap <name>` | open the worktree in the IDE (`-w` for Ghostty) |
| `kit diff` | diff the worktree vs master |
| `kit submit` | push the branch via `gt submit` |
| `kit wash [--merged]` | strip a kit (bulk-wash merged/closed) |

Full reference: the repo README. If they hit trouble, `kit doctor` first,
then `kit <command> --help`.
