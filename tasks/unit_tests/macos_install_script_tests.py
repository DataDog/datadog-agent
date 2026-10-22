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
        postinst = (REPO_ROOT / "omnibus/package-scripts/agent-dmg/postinst").read_text()
        # Execute only the real input/staging/config blocks, never package installation or service setup.
        self.stage = installer[installer.index("gui_app_menu_enabled=false") : installer.index("# Optional: Chrome")]
        self.stage += installer[
            installer.index("# Write configuration for the pkg") : installer.index("# Obtain the DMG")
        ]
        self.configure = postinst[
            postinst.index("# Read configuration written") : postinst.index("install_info_content=")
        ]
        if sys.platform != "darwin":
            # GNU sed takes no separate empty argument for an in-place edit without backup.
            self.configure = self.configure.replace("sed -i ''", "sed -i")

    def stage_logs_enabled(self, value):
        env = {
            "PATH": "/usr/bin:/bin",
            "install_staging_dir": str(self.env_file.parent),
            "install_env_file": str(self.env_file),
        }
        if value is not None:
            env["DD_LOGS_ENABLED"] = value
        return subprocess.run(["/bin/bash", "-ec", self.stage], cwd=self.root, env=env, capture_output=True, text=True)

    def configure_logs_enabled(self):
        result = subprocess.run(
            ["/bin/sh", "-c", self.configure],
            cwd=self.root,
            env={"PATH": "/usr/bin:/bin", "CONF_DIR": str(self.root), "INSTALL_ENV_FILE": str(self.env_file)},
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stderr, "")

    def test_logs_enabled_forwarding_and_config(self):
        for value in ("true", "false", "", None):
            for existing in ("# logs_enabled: false\n", "logs_enabled: false\n", "logs_enabled: true\n", ""):
                with self.subTest(value=value, existing=existing):
                    original = "site: datadoghq.com\n" + existing
                    self.config.write_text(original)
                    result = self.stage_logs_enabled(value)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    forwarded = [
                        line for line in self.env_file.read_text().splitlines() if line.startswith("DD_LOGS_ENABLED=")
                    ]
                    self.assertEqual(forwarded, [f"DD_LOGS_ENABLED={value}"] if value else [])
                    self.configure_logs_enabled()
                    expected = original
                    if value:
                        expected = "site: datadoghq.com\n" + ("" if existing else "\n") + f"logs_enabled: {value}\n"
                    self.assertEqual(self.config.read_text(), expected)

    def test_logs_enabled_rejects_invalid_install_input(self):
        for value in INVALID_LOGS_ENABLED:
            with self.subTest(value=value):
                result = self.stage_logs_enabled(value)
                self.assertEqual(result.returncode, 1)
                self.assertIn("DD_LOGS_ENABLED must be true or false", result.stdout)
                self.assertFalse(self.env_file.exists())
                self.assertFalse((self.root / "injected").exists())

    def test_logs_enabled_ignores_invalid_staged_input(self):
        self.env_file.parent.mkdir()
        for value in INVALID_LOGS_ENABLED:
            with self.subTest(value=value):
                original = "logs_enabled: true\n"
                self.config.write_text(original)
                self.env_file.write_text(f"DD_LOGS_ENABLED={value}\n")
                self.configure_logs_enabled()
                self.assertEqual(self.config.read_text(), original)
                self.assertFalse((self.root / "injected").exists())

    def test_logs_enabled_preserves_config_without_staging_file(self):
        original = "logs_enabled: true\n"
        self.config.write_text(original)
        self.configure_logs_enabled()
        self.assertEqual(self.config.read_text(), original)
