import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
INVALID_LOGS_ENABLED = ["TRUE", "1", "yes", "false\nDD_LOGS_ENABLED=true", "$(touch injected)", "true/; touch injected"]


@unittest.skipIf(sys.platform == "win32", "macOS install scripts require a POSIX shell")
class TestMacOSInstallScript(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env_file = self.root / "staging/env"
        self.config = self.root / "datadog.yaml"
        installer = (REPO_ROOT / "cmd/agent/macos/install_mac_os.sh").read_text()
        # Execute only the real input/staging blocks, never package installation or service setup.
        # How the staged values land in datadog.yaml is the installer's postinst hook, tested in
        # pkg/fleet/installer/packages/datadog_agent_darwin_dmg_test.go.
        self.stage = installer[installer.index("gui_app_menu_enabled=false") : installer.index("# Optional: Chrome")]
        self.stage += installer[
            installer.index("# Write configuration for the pkg") : installer.index("# Obtain the DMG")
        ]

    def stage_logs_enabled(self, value):
        env = {
            "PATH": "/usr/bin:/bin",
            "install_staging_dir": str(self.env_file.parent),
            "install_env_file": str(self.env_file),
        }
        if value is not None:
            env["DD_LOGS_ENABLED"] = value
        return subprocess.run(["/bin/bash", "-ec", self.stage], cwd=self.root, env=env, capture_output=True, text=True)

    def test_logs_enabled_forwarding(self):
        for value in ("true", "false", "", None):
            with self.subTest(value=value):
                result = self.stage_logs_enabled(value)
                self.assertEqual(result.returncode, 0, result.stderr)
                forwarded = [
                    line for line in self.env_file.read_text().splitlines() if line.startswith("DD_LOGS_ENABLED=")
                ]
                self.assertEqual(forwarded, [f"DD_LOGS_ENABLED={value}"] if value else [])

    def test_logs_enabled_rejects_invalid_install_input(self):
        for value in INVALID_LOGS_ENABLED:
            with self.subTest(value=value):
                result = self.stage_logs_enabled(value)
                self.assertEqual(result.returncode, 1)
                self.assertIn("DD_LOGS_ENABLED must be true or false", result.stdout)
                self.assertFalse(self.env_file.exists())
                self.assertFalse((self.root / "injected").exists())
