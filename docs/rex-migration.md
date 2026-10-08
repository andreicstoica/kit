# Herdr to Rex migration

`kit migrate rex` (or `kit migrate rex --dry-run`) is designed to import the saved workspace structure while
leaving Herdr installed and untouched. The default invocation is a dry run;
`--apply` is required before any Rex workspace is created.

Use `--workspace <Herdr-label-or-ID>` to isolate one workspace. The importer
uses Herdr's public workspace/tab/pane snapshot: IDs, labels, checkout and
pane working directories, and agent identity. It does not read
terminal output or replay pane commands. Rex receives newly created shells;
Herdr's live processes and agent sessions cannot be transferred. Review the
plan and resume agents manually in Rex. Do not resume an agent or service in
Rex while its Herdr counterpart is still running, or duplicate work may occur.

Herdr's public snapshot does not include pane split direction or proportions;
reconstructed multi-pane layouts use horizontal splits and are only a
best-effort geometry match. Imports must preserve existing Rex sessions. A persisted source-ID mapping
makes retries idempotent; an existing destination without a matching mapping
is treated as a collision and skipped rather than overwritten. Missing
directories, unknown agents, and pane working directories outside the
workspace checkout are reported for review. Logs are not automatically
attached; any log-tail tabs must be an explicit, safe opt-in.
Source workspace/tab/pane IDs and their returned Rex IDs are stored in
`<KIT_STATE_DIR>/rex-import.json` (normally under `~/.config/kit/`) with mode
`0600`; no commands, prompts, or terminal contents are saved.

No migration should close Herdr, kill its server, or claim to transfer its live
PTYs. Keep Herdr available until the imported Rex layout has been reviewed and
the user has manually resumed only the intended agents/services.

## Trial configuration and fonts

Keep an older installed Kit's config writes separate from this branch. The
`dev/kit-rex` launcher uses `~/.config/kit-rex` for config and the existing
`~/.config/kit/run` directory for service PID/log files. It never installs the
branch over the released `kit` binary.

Ordinary `close` cleans all mapped terminal backends. During a trial, use
`close --backend rex` to leave Herdr's original processes running. Imported
secondary sessions sharing a checkout are recorded and included in Rex cleanup.
Historical import IDs are retained; an explicitly closed destination is not
silently recreated by another import. Use normal `open` to create a fresh
workspace, or resolve the historical import mapping explicitly.

The current Rex build has no verified documented override for separate regular,
bold, italic, and bold-italic font families. Ghostty's Geist Mono body plus Rec
Mono Duotone emphasis setup has not been applied through guessed preference keys.
Existing Rex font preferences remain unchanged.
