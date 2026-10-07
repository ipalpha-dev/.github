#!/usr/bin/env python3
"""Exercise the real Bash setup UI in a controlling terminal, without installs."""
import errno
import fcntl
import os
from pathlib import Path
import pty
import select
import signal
import struct
import subprocess
import tempfile
import termios
import time
import unittest


ROOT = str(Path(__file__).resolve().parent.parent)
PRELUDE = """
set -euo pipefail
source lib/i18n.sh
source lib/setup-ui.sh
ipalpha_ui_start
trap ipalpha_ui_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
"""


class Terminal:
    def __init__(self, script, rows=24, cols=80, plain=False):
        self.pid, self.fd = pty.fork()
        if self.pid == 0:
            os.chdir(ROOT)
            os.environ["TERM"] = "xterm-256color"
            os.environ["IPALPHA_PLAIN"] = "1" if plain else "0"
            os.execv("/bin/bash", ["bash", "-c", script])
        fcntl.ioctl(self.fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
        self.initial_attrs = termios.tcgetattr(self.fd)
        self.output = b""
        self.status = None

    def until(self, marker, timeout=8, count=1):
        deadline = time.monotonic() + timeout
        while self.output.count(marker) < count and time.monotonic() < deadline:
            if select.select([self.fd], [], [], 0.1)[0]:
                try:
                    chunk = os.read(self.fd, 65536)
                except OSError as error:
                    if error.errno == errno.EIO:
                        break
                    raise
                if not chunk:
                    break
                self.output += chunk
        if self.output.count(marker) < count:
            raise AssertionError(f"Missing {marker!r} in {self.output!r}")

    def send(self, value):
        os.write(self.fd, value)

    def finish(self, expected=0):
        self.until(b"\x1b[?1049l")
        deadline = time.monotonic() + 8
        while time.monotonic() < deadline:
            # macOS can defer terminal-process exit until queued output is drained.
            if select.select([self.fd], [], [], 0.05)[0]:
                try:
                    self.output += os.read(self.fd, 65536)
                except OSError as error:
                    if error.errno != errno.EIO:
                        raise
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                self.status = status
                break
            time.sleep(0.05)
        if self.status is None:
            raise AssertionError("UI did not exit")
        actual = os.waitstatus_to_exitcode(self.status)
        if actual != expected:
            raise AssertionError(f"Exit {actual}, expected {expected}: {self.output!r}")
        if termios.tcgetattr(self.fd) != self.initial_attrs:
            raise AssertionError("Terminal settings were not restored")

    def close(self):
        os.close(self.fd)
        if self.status is None:
            try:
                os.kill(self.pid, signal.SIGKILL)
                os.waitpid(self.pid, 0)
            except ProcessLookupError:
                pass


class SetupUI(unittest.TestCase):
    def terminal(self, script, **kwargs):
        terminal = Terminal(PRELUDE + script, **kwargs)
        self.addCleanup(terminal.close)
        return terminal

    def test_cross_style_panel_and_language_arrows(self):
        terminal = self.terminal("""
ipalpha_prompt_language
[[ "$ipalpha_lang" == en-US ]]
ipalpha_ui_stop
""")
        terminal.until(b"English")
        self.assertIn(b"\x1b[?1049h", terminal.output)
        self.assertIn(b"\x1b[38;5;33m", terminal.output)
        self.assertIn(b"\x1b[1;38;5;15;48;5;33m IPAlpha - Setup", terminal.output)
        self.assertIn("╭".encode(), terminal.output)
        self.assertIn("╰".encode(), terminal.output)
        self.assertIn(b"\x1b[1;38;5;39m", terminal.output)
        self.assertNotIn(b"\x1b[97;44m", terminal.output)
        self.assertIn(b"\x1b[12;1H", terminal.output)
        terminal.send(b"\x1b[B\r")
        terminal.finish()

    def test_numeric_selection_and_default_input(self):
        terminal = self.terminal("""
ipalpha_ui_select Test Choice First Second Third
[[ "$ipalpha_ui_answer" == 3 ]]
ipalpha_ui_input Folder Path '/tmp/workspace with spaces'
[[ "$ipalpha_ui_answer" == '/tmp/workspace with spaces' ]]
ipalpha_ui_stop
""")
        terminal.until(b"Third")
        terminal.send(b"3")
        terminal.until(b"/tmp/workspace with spaces")
        terminal.send(b"\r")
        terminal.finish()

    def test_input_custom_path(self):
        terminal = self.terminal("""
ipalpha_ui_input Folder Path /tmp/default
[[ "$ipalpha_ui_answer" == '/tmp/custom path' ]]
ipalpha_ui_stop
""")
        terminal.until(b"/tmp/default")
        terminal.send(b"\x15/tmp/custom path\r")
        terminal.finish()

    def test_input_default_is_editable_in_place(self):
        terminal = self.terminal("""
ipalpha_ui_input Folder Path /tmp/default
[[ "$ipalpha_ui_answer" == '/tmp/default-edited' ]]
ipalpha_ui_stop
""")
        terminal.until(b"/tmp/default")
        terminal.send(b"-editd\x1b[De\r")
        terminal.finish()

    def test_escape_cancel_restores_terminal(self):
        terminal = self.terminal("ipalpha_prompt_language")
        terminal.until(b"English")
        terminal.send(b"\x1b")
        terminal.finish(expected=130)

    def test_ctrl_c_restores_terminal(self):
        terminal = self.terminal("ipalpha_ui_input Folder Path /tmp/default")
        terminal.until(b"/tmp/default")
        terminal.send(b"\x03")
        terminal.finish(expected=130)

    def test_superuser_name_default_and_phone_validation(self):
        terminal = self.terminal("""
source lib/common.sh
source lib/env.sh
source lib/superuser.sh
ipalpha_i18n_init en-US
unset IPALPHA_SUPERUSER_NAME IPALPHA_SUPERUSER_PHONE
ipalpha_prompt_superuser /tmp/unused-superuser-qa-root
[[ "$ipalpha_superuser_name" == 'Joao Silva Costa' ]]
[[ "$ipalpha_superuser_phone" == '+5599900000000' ]]
ipalpha_ui_stop
""")
        terminal.until(b'Joao Silva Costa')
        terminal.send(b'\r')
        terminal.until(b'Your mobile number with area code')
        terminal.until(b'\x1b[?25h', count=2)
        terminal.send(b'123\r')
        terminal.until(b'11 digits required')
        terminal.until(b'\x1b[?25h', count=6)
        terminal.send(b'\x1599900000000\r')
        terminal.finish()

    def test_progress_keeps_function_state_and_stops_on_failure(self):
        terminal = self.terminal("""
update_state() { ipalpha_test_state=changed; echo 'Task output'; }
ipalpha_ui_run Progress update_state
[[ "$ipalpha_test_state" == changed ]]
fail_task() { echo 'Expected failure'; false; echo 'MUST NOT RUN'; }
ipalpha_ui_run Failure fail_task
""")
        terminal.finish(expected=1)
        self.assertNotIn(b"MUST NOT RUN", terminal.output)

    def test_progress_updates_without_clearing_or_repainting_idle_content(self):
        terminal = self.terminal("""
echo FIRST-LINE
ipalpha_ui_progress Cloning
ipalpha_ui_progress Cloning
ipalpha_ui_progress Cloning
echo SECOND-LINE
ipalpha_ui_progress Cloning
ipalpha_ui_progress Cloning
ipalpha_ui_stop
""")
        terminal.finish()
        self.assertEqual(terminal.output.count(b"\x1b[2J"), 1)
        self.assertEqual(terminal.output.count(b"FIRST-LINE"), 1)
        self.assertEqual(terminal.output.count(b"SECOND-LINE"), 1)

    def test_background_progress_does_not_clear_on_every_tick(self):
        terminal = self.terminal("""
slow_work() { echo START; sleep 1; echo FINISH; }
ipalpha_ui_run Cloning slow_work
ipalpha_ui_stop
""")
        terminal.finish()
        self.assertEqual(terminal.output.count(b"\x1b[2J"), 1)
        self.assertIn(b"FINISH", terminal.output)

    def test_repeated_progress_stage_reuses_the_panel(self):
        terminal = self.terminal("""
first_work() { echo FIRST-STAGE; }
second_work() { echo SECOND-STAGE; }
ipalpha_ui_run Configuring first_work
ipalpha_ui_run Configuring second_work
ipalpha_ui_stop
""")
        terminal.finish()
        self.assertEqual(terminal.output.count(b"\x1b[2J"), 1)
        self.assertIn(b"SECOND-STAGE", terminal.output)

    def test_parallel_clone_cancel_restores_terminal(self):
        terminal = self.terminal("""
source lib/common.sh
source lib/clone.sh
ipalpha_all_repos() { printf '%s\\n' alpha beta; }
ipalpha_clone_repo() { sleep 30; }
ipalpha_ui_run Cloning ipalpha_clone_org_repos /tmp/unused-qa-target
""")
        terminal.until(b"beta")
        terminal.send(b"\x03")
        terminal.finish(expected=130)

    def test_wide_screen(self):
        terminal = self.terminal("ipalpha_prompt_language", rows=40, cols=140)
        terminal.until(b"English")
        # Cross keeps the wizard panel at 80 columns even in a wide terminal.
        self.assertIn(b"\x1b[2;80H", terminal.output)
        self.assertNotIn(b"\x1b[2;140H", terminal.output)
        terminal.send(b"\r")
        terminal.finish()

    def test_complete_interactive_setup_without_installing_tools(self):
        with tempfile.TemporaryDirectory(prefix="ipalpha-ui-test-") as fixture:
            terminal = self.terminal("""
ipalpha_ui_stop
ipalpha_ui_cleanup
export IPALPHA_TEST_NO_PORT_PROBE=1
export IPALPHA_CLONE_COMMAND="$PWD/tests/fake-clone"
export IPALPHA_SKIP_INSTALL=1 IPALPHA_NO_SHELL=1
export IPALPHA_SUPERUSER_NAME='QA Developer'
export IPALPHA_SUPERUSER_PHONE=99900000000
unset IPALPHA_LANG IPALPHA_TARGET_DIR
exec ./setup --skip-tools --keep-setup
""")
            terminal.until(b"English")
            terminal.send(b"2")
            terminal.until(b"\x1b[11;4H")
            target = fixture + "/workspace with spaces"
            terminal.send(b"\x15" + target.encode() + b"\r")
            terminal.until(b"Setup complete", timeout=30)
            terminal.finish()
            self.assertTrue(Path(target, ".ipalpha/settings").is_file())
            self.assertTrue(Path(target, "run").is_file())

    def test_redirected_and_plain_mode_have_no_screen_controls(self):
        for plain in ("0", "1"):
            result = subprocess.run(
                ["/bin/bash", "-c", PRELUDE + "ipalpha_ui_run Test echo ordinary-output"],
                cwd=ROOT, env={**os.environ, "TERM": "xterm", "IPALPHA_PLAIN": plain},
                capture_output=True, check=True,
            )
            self.assertEqual(result.stdout, b"ordinary-output\n")
            self.assertEqual(result.stderr, b"")


if __name__ == "__main__":
    unittest.main()
