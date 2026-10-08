"""Exercise the real menu in light/dark PTYs without selecting any command.

Usage: python3 dev/pty-smoke.py [dist/kit-rex]
Requires macOS/Unix; uses only the Python standard library.
"""

import fcntl
import os
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time


def smoke(binary, theme):
    with tempfile.TemporaryDirectory(prefix="kit-pty-") as state:
        with open(os.path.join(state, "config.toml"), "w") as config:
            config.write("schema = 2\n")
        master, slave = pty.openpty()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 100, 0, 0))
        env = dict(os.environ, KIT_STATE_DIR=state, TERM="xterm-256color", COLORTERM="truecolor")
        proc = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave,
                                env=env, start_new_session=True)
        os.close(slave)
        start = time.monotonic()
        output = b""
        first = None
        resized = cancelled = False
        try:
            while proc.poll() is None and time.monotonic() - start < 4:
                ready, _, _ = select.select([master], [], [], 0.05)
                if ready:
                    try:
                        chunk = os.read(master, 65536)
                    except OSError:
                        break
                    output += chunk
                    if b"\x1b]11;?" in chunk:
                        rgb = b"ffff/ffff/ffff" if theme == "light" else b"0000/0000/0000"
                        os.write(master, b"\x1b]11;rgb:" + rgb + b"\x07")
                    if first is None and b"lineup" in output:
                        first = time.monotonic() - start
                elapsed = time.monotonic() - start
                if elapsed > 0.3 and not resized:
                    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 24, 40, 0, 0))
                    proc.send_signal(signal.SIGWINCH)
                    os.write(master, b"\x1b[B\x1b[A")
                    resized = True
                if elapsed > 0.8 and not cancelled:
                    os.write(master, b"\x1b")
                    cancelled = True
            code = proc.wait(timeout=2)
            if first is None or code != 0:
                raise AssertionError((theme, code, output[-500:]))
            print(f"{theme}: menu {first * 1000:.0f}ms; resize/navigation/cancel OK")
        finally:
            if proc.poll() is None:
                proc.kill()
                proc.wait()
            os.close(master)


if __name__ == "__main__":
    for appearance in ("light", "dark"):
        smoke(sys.argv[1] if len(sys.argv) > 1 else "./dist/kit-rex", appearance)
