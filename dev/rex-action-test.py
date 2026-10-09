"""Run with python3 dev/rex-action-test.py; never invokes the real Kit binary."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


class RexActionTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="kit-palette-test-")
        self.addCleanup(self.temp.cleanup)
        self.kit = Path(self.temp.name) / "fake kit"
        self.kit.write_text('#!/bin/sh\nprintf "ARG=<%s>\\n" "$@"\nexit "${FAKE_KIT_EXIT:-0}"\n')
        self.kit.chmod(0o700)
        self.env = dict(os.environ, KIT_REX_BIN=str(self.kit))
        self.runner = Path(__file__).with_name("rex-action")

    def run_action(self, action, text="", code=0, session=""):
        return subprocess.run(["/bin/sh", str(self.runner), action, session], input=text,
                              env=dict(self.env, FAKE_KIT_EXIT=str(code)),
                              text=True, capture_output=True, timeout=3)

    def test_successful_wizard_does_not_wait_for_an_extra_key(self):
        result = self.run_action("design")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "ARG=<design>\n")

    def test_read_only_status_waits_for_acknowledgment(self):
        result = self.run_action("agents", "\n")
        self.assertEqual(result.returncode, 0)
        self.assertIn("ARG=<lineup>\nARG=<--agents>", result.stdout)
        self.assertIn("Press Enter", result.stdout)

    def test_failure_keeps_the_error_visible_until_acknowledged(self):
        result = self.run_action("design", "\n", code=1)
        self.assertEqual(result.returncode, 1)
        self.assertIn("Press Enter", result.stdout)

    def test_session_identity_is_one_literal_argument_without_a_picker(self):
        result = self.run_action("park", session="session; echo injected")
        self.assertEqual(result.returncode, 0)
        self.assertIn("ARG=<rex>\nARG=<action>\nARG=<park>\nARG=<--session>\nARG=<session; echo injected>", result.stdout)
        self.assertNotIn("Workspace name", result.stdout)

    def test_missing_session_never_parks_any_workspace(self):
        result = self.run_action("park", "\n")
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("ARG=<park>", result.stdout)

    def test_direct_failure_exits_without_a_hidden_acknowledgment_prompt(self):
        result = self.run_action("park", code=1, session="session1")
        self.assertEqual(result.returncode, 1)
        self.assertNotIn("Press Enter", result.stdout)

    def test_service_actions_use_the_full_cli_picker_and_keep_output_visible(self):
        for action in ("play", "pause"):
            result = self.run_action(action, "\n", session="unmapped-session")
            self.assertEqual(result.returncode, 0)
            self.assertIn("ARG=<" + action + ">\n", result.stdout)
            self.assertNotIn("ARG=<rex>", result.stdout)
            self.assertNotIn("unmapped-session", result.stdout)
            self.assertIn("Press Enter", result.stdout)

    def test_sync_retains_the_interactive_flow_and_output(self):
        result = self.run_action("sync", "\n")
        self.assertEqual(result.returncode, 0)
        self.assertIn("ARG=<sync>", result.stdout)
        self.assertIn("Press Enter", result.stdout)

    def test_restart_uses_the_cli_and_keeps_results_visible(self):
        result = self.run_action("restart", "\n")
        self.assertEqual(result.returncode, 0)
        self.assertIn("ARG=<restart>\n", result.stdout)
        self.assertIn("Press Enter", result.stdout)

    def test_unknown_action_never_invokes_kit(self):
        result = self.run_action("typo")
        self.assertEqual(result.returncode, 2)
        self.assertNotIn("ARG=", result.stdout)


if __name__ == "__main__":
    unittest.main()
