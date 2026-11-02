"""Test GUI-session gates without installing files or changing host services."""

import subprocess
import sys
import unittest
from pathlib import Path

POSTINST = Path(__file__).resolve().parents[2] / "omnibus/package-scripts/agent-dmg/postinst"


@unittest.skipIf(sys.platform == "win32", "macOS package hooks require a POSIX shell")
class TestMacOSGUISessionGate(unittest.TestCase):
    def run_startup(self, service, session_exists):
        script = POSTINST.read_text()
        if service == "gui":
            start = script.index("# Load GUI immediately")
            end = script.index("# Remove previous LaunchAgent labels", start)
        else:
            start = script.index('    if [ -n "$CONSOLE_USER_UID" ]', script.index("# Install AI Usage Agent"))
            end = script.index(
                '\nelse\n    echo "WARNING: AI Usage Agent desktop monitor LaunchAgent template missing', start
            )
        # Model a root installer outside Aqua, with or without a console GUI session.
        mocks = """
stat() { echo alice; }
id() { echo 501; }
sudo() { echo System; }
launchctl() {
    case "$1" in
        print) [ "$2" = "gui/501" ] && [ "$SESSION_EXISTS" = yes ] ;;
        enable|bootstrap) printf 'called: %s\\n' "$*" ;;
        *) return 1 ;;
    esac
}
"""
        return subprocess.run(
            ["/bin/sh", "-c", mocks + script[start:end]],
            env={
                "PATH": "/usr/bin:/bin",
                "SESSION_EXISTS": "yes" if session_exists else "no",
                "CONSOLE_USER": "alice",
                "CONSOLE_USER_UID": "501",
                "AI_USAGE_DESKTOP_MONITOR_LABEL": "com.datadoghq.ai-usage-agent.desktop-monitor",
                "AI_USAGE_DESKTOP_MONITOR_PLIST": "/Library/LaunchAgents/com.datadoghq.ai-usage-agent.desktop-monitor.plist",
            },
            capture_output=True,
            text=True,
            timeout=15,
        )

    def test_existing_gui_session_starts_helpers_from_non_aqua_installer(self):
        for service, label in (
            ("gui", "com.datadoghq.gui"),
            ("desktop-monitor", "com.datadoghq.ai-usage-agent.desktop-monitor"),
        ):
            with self.subTest(service=service):
                result = self.run_startup(service, session_exists=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(f"called: enable gui/501/{label}", result.stdout)
                self.assertIn(f"called: bootstrap gui/501 /Library/LaunchAgents/{label}.plist", result.stdout)

    def test_absent_gui_session_defers_helpers_until_login(self):
        for service in ("gui", "desktop-monitor"):
            with self.subTest(service=service):
                result = self.run_startup(service, session_exists=False)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertNotIn("called:", result.stdout)
                self.assertIn("will launch at next user login", result.stdout)
