---
name: kit-kickoff
description: Use when asked to implement, build, or fix a task in Liftoff, especially in a new workspace. Choose the right kit worktree and necessary resources, safely hand off an existing fx thread, or, when the user wants it, launch a fresh Claude Code agent there.
---

# Kit kickoff

## Inspect before choosing

- Read the repository instructions and relevant source/tests, not just the user prompt. Check `pwd`, `git status --short`, `git branch --show-current`, `git rev-parse --show-toplevel`, `git worktree list`, and `kit lineup`.
- For read-only questions, stay put; do not create a worktree. In an existing feature worktree, continue there unless the user requests another. Never move, stash, reset, or copy dirty edits into a different worktree automatically.
- For a new implementation workspace, choose a short task-specific name and the smallest service set based on the inspected code. Do not start services merely because a worktree was created.

## Create without UI

Always use the non-interactive JSON design path, never the wizard or editor/terminal-opening helpers. For routine implementation, create directly with safe defaults:

```sh
kit design NAME --non-interactive --json --db none --backend-deps=false
```

Only when the user asks for a plan, add `--dry-run` to return JSON without side effects instead of creating the worktree.

Replace `NAME` with the selected name. Defaults are no database, no backend dependency install, frontend dependency symlinks enabled, and Graphite tracking when available. Use `--symlink-frontend=false` when package manifests or frontend dependencies will change, so installs cannot modify master's shared packages. Use `--graphite=false` when tracking is inappropriate. Install backend dependencies only when the task needs them (`--backend-deps=true`). Design does not mean starting services.

Read the returned JSON and verify the actual worktree/cwd, branch, and paths with read-only checks. Target **every subsequent command** at that returned cwd (or the relevant subdirectory); use explicit tool cwd/absolute paths. Do not guess paths from the name. A shell `cd` changes that shell only: it does not relocate the current agent runtime or its tool defaults.

## Add only required resources

- Choose `--db none` unless the task actually requires PostgreSQL. Frontend-only and static work often need none.
- Use `--db empty` for fixture-based tests. Run the repository's documented migrations yourself **inside the feature worktree**, with its database configuration. Never use a master-worktree migration helper for feature migrations.
- Use `--db copy` only when realistic existing data is necessary. Copy may use a PostgreSQL template. Never authorize a full `pg_dump` implicitly: get explicit user permission before passing `--allow-pg-dump`. This applies both during design and later.
- A database can be added later: `kit db NAME --mode empty` or `kit db NAME --mode copy`. Add `--allow-pg-dump` only after permission; do not silently fall back to a full dump on failure.
- Start only what the inspected task needs, when needed: for example `kit play NAME --only app`, `--only app,api`, or `--only admin,admin_be`. Workers (`celery`, `beat`) and MCP are opt-in task needs, not automatic startup. Use `kit links NAME` for allocated URLs and verify the selected services. Run these from the verified worktree cwd.

## Continue here or transfer the thread

Continuing with explicit cwd is valid. Say which worktree commands target; never claim the runtime moved just because a worktree was created or a shell changed directory.

For an actual existing-thread transfer, prefer a native runtime switch if this client provides one. Otherwise obtain the exact current fx session ID from runtime/session metadata; do not invent it or select another thread. From the verified worktree cwd, `kit enter NAME --session FX_ID --print` prints the foreground resume command without launching fx and is the only allowed enter mode from a non-interactive agent command.

End the current turn with the handoff instructions. The user must quit the original fx process **before** running `kit enter NAME --session FX_ID` in their terminal. That command resumes fx in the foreground. Never launch a second writer for the same thread or background the resumed agent. Do not call unprinted `kit enter` from the active agent.

## Launch a fresh agent thread (only when authorized)

Use this only when the user asks for a new agent in the worktree, or approves one when offered. It starts a **new** thread; it does not transfer or resume the current one. The current thread keeps running, so do not edit the same worktree from both threads.

1. Create or choose the worktree first with the headless `kit design` path above, and add only the resources the task needs. `kit agent launch` never creates worktrees, databases, or services.
2. Write a bounded brief (at most 32 KiB of UTF-8 text) with the goal, the verified worktree path, the chosen resources, constraints, and what is already done. Do not put secrets in it. It must not start with `-`.
3. Run `kit agent launch NAME --brief FILE` (or `--brief -` to read stdin). Use `--harness` and `--model` if the user specifies a client or model. Kit defaults to Claude Code with Opus and supports `fx`, `codex`, and `opencode`. OpenCode uses its configured model. Kit sends the brief as the first prompt and selects the new window in Rex. Kit never passes a permission-bypass flag or prints the brief.
4. Read the result. Exit 0 means the agent window opened. A warning that Rex did not select it means the window is still open: tell the user to select the `agent` window in the Rex app. An error that says Kit could not confirm the launch means a window may exist: ask the user to check the session before any retry. Other errors mean no window opened. Never launch twice for one brief without checking.

Kit refuses parked or cleanup-pending worktrees, `master`, and unknown names. Fix the cause (for example `kit resume NAME`) instead of working around it.

## Startup and fallback

- One-time installation is necessary for automatic discovery: `kit agent install` writes `~/.fx/skills/kit-kickoff/SKILL.md`. For another client, `kit agent install --path /absolute/client/skills/kit-kickoff` takes the complete skill directory. Reload/start the client to discover it. Do not install without the user's request; differing files require explicit `--force`.
- Without installation, `kit agent` prints these instructions; read and follow them in the current thread. If kit or the required headless flags are missing, check `kit --version`, `kit design --help`, and `kit doctor`, then report the blocker or ask the user to update/setup kit. Do not substitute an interactive wizard, a raw worktree/database clone, or an unauthorized or hand-built agent process. The only allowed second agent is an authorized `kit agent launch`.
- If native switching is unavailable or the session ID is unknown, continue using explicit cwd, or stop with the exact missing handoff step. Never claim a thread transfer happened without runtime evidence.
