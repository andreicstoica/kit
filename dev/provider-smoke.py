"""Verify installed kit/kit-herdr provider routing without contacting real runtimes."""
import os
from pathlib import Path
import subprocess
import tempfile


repo = Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix="kit-provider-") as temp:
    root = Path(temp)
    prefix = root / "installed"
    subprocess.run(["make", "install", "PREFIX=" + str(prefix)], cwd=repo, check=True)
    state = root / "state"
    state.mkdir()
    (root / "master" / ".git").mkdir(parents=True)
    (root / "feature").mkdir()
    config = state / "config.toml"
    fixture = 'schema = 2\n[settings]\nroot = "' + str(root) + '"\nmaster_dir = "master"\n'
    config.write_text(fixture)
    fake_bin = root / "fake-bin"
    fake_bin.mkdir()
    capture = root / "provider-calls"
    for name in ("rex", "herdr"):
        binary = fake_bin / name
        binary.write_text('#!/bin/sh\nprintf "' + name + '\\n" >> "$PROVIDER_CAPTURE"\nexit 1\n')
        binary.chmod(0o700)
    env = dict(os.environ, KIT_STATE_DIR=str(state), KIT_RUN_DIR=str(root / "run"),
               KIT_ROOT=str(root), KIT_MASTER_DIR="master", KIT_WORKSPACE_BACKEND="",
               PROVIDER_CAPTURE=str(capture), PATH=str(fake_bin) + os.pathsep + os.environ["PATH"])
    assert (prefix / "bin" / "kit-herdr").is_symlink()
    cases = [("kit", "", "", "rex"), ("kit-herdr", "rex", "", "herdr"),
             ("kit", "herdr", "", "herdr"), ("kit-herdr", "rex", "rex", "rex"),
             ("kit", "", "invalid", None)]
    for invocation, setting, override, expected in cases:
        config.write_text(fixture + ('workspace_backend = "' + setting + '"\n' if setting else ""))
        before = config.read_bytes()
        capture.write_text("")
        result = subprocess.run([str(prefix / "bin" / invocation), "focus", "feature", "--no-attach", "--layout", "simple"],
                                env=dict(env, KIT_WORKSPACE_BACKEND=override), capture_output=True, text=True, timeout=10)
        assert result.returncode != 0, result
        calls = capture.read_text().splitlines()
        assert (calls and set(calls) == {expected}) if expected else not calls, (invocation, calls, result.stderr)
        assert config.read_bytes() == before, "Provider routing changed workspace metadata"
    print("VERIFIED: installed kit defaults to Rex; kit-herdr overrides saved Rex; explicit config/env work; invalid overrides touch neither runtime.")
