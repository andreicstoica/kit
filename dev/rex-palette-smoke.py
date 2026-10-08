"""Test the native Restart tab with fake Kit; never restarts real services."""
import json
from pathlib import Path
import subprocess
import tempfile
import time

CLI = "/Applications/Rex Beta.app/Contents/Helpers/rex"


def call(*args):
    return subprocess.check_output([CLI, "--autostart=false", *args], text=True, timeout=10)


def block_call(sid, bid, method, **args):
    payload = dict(args, session_id=sid, block_id=bid)
    lua = 'local r,e=rex.block.call("com.superlogical.terminal",rex.args.method,rex.args.payload); if e then error(e,0) end; return r'
    return json.loads(call("do", "-s", sid, "-e", lua, "--args", json.dumps(dict(method=method, payload=payload))))


with tempfile.TemporaryDirectory(prefix="kit-palette-live-") as cwd:
    directory = Path(cwd)
    kit = directory / "fake-kit"
    kit.write_text('#!/bin/sh\n[ "$1" = restart ] || exit 1\nprintf "Restart fixture completed\\n"\n')
    kit.chmod(0o700)
    source = Path(__file__).with_name("rex-actions.lua").read_text()
    source = source.replace('local kit = os.getenv("KIT_REX_BIN") or home .. "/.local/bin/kit"',
                            "local kit = " + json.dumps(str(kit)))
    actions = directory / "rex-actions.lua"
    actions.write_text(source)
    (directory / "rex-action").write_text(Path(__file__).with_name("rex-action").read_text())
    sid = json.loads(call("new", Path(cwd).name, "--cwd", cwd, "--window", "scratch", "--json"))["session_id"]
    try:
        result = json.loads(call("do", "-s", sid, str(actions), "--action", "kit_restart"))
        bid = result["block_ids"][0]
        assert result.get("window_id"), "Kit action must use a native tab, not a transparent floating layer"
        assert not result.get("layer_id")
        block_call(sid, bid, "resize", rows=40, columns=140)
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            if "Press Enter" in block_call(sid, bid, "format", format="text")["content"]:
                break
            time.sleep(0.1)
        else:
            raise AssertionError("Kit restart did not reach its acknowledgment prompt: " + block_call(sid, bid, "format", format="text")["content"])
        block_call(sid, bid, "write", data="\r")
        deadline = time.monotonic() + 6
        while time.monotonic() < deadline:
            blocks = json.loads(call("do", "-s", sid, "-e", "local b,e=rex.session.list_blocks{}; if e then error(e,0) end; return b"))["blocks"]
            if not any(b["block_id"] == bid for b in blocks):
                break
            time.sleep(0.1)
        else:
            raise AssertionError("Kit panel did not close after Enter")
        assert len(blocks) == 1, blocks
        windows = json.loads(call("do", "-s", sid, "-e", "return rex.session.view{}"))["windows"]
        assert len(windows) == 1 and windows[0]["label"] == "scratch", windows
        print("Kit native tab rendered and closed on Enter; original tab and pane preserved.")
    finally:
        call("kill", sid)
