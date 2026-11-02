"""Exercise package-hook GUI lifecycle code without touching the host's launchd."""

import json
import re
import shlex
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
HOOKS = REPO_ROOT / "omnibus/package-scripts/agent-dmg"
LABEL = "com.datadoghq.gui"

# Stateful fake launchd. Only the shell functions/GUI blocks extracted below run;
# package payload installation, real sudo, and host service management never run.
LAUNCHCTL = r'''
import json
import os
import sys
from pathlib import Path

path = Path(os.environ["LAUNCHD_STATE"])
state = json.loads(path.read_text())
args = sys.argv[1:]
state.setdefault("calls", []).append(args)
command, target = args[:2]
uid = target.split("/")[1]
label = target.split("/")[2] if target.count("/") == 2 else ""
service = uid + "/" + label
rc = 0
if uid not in state["domains"]:
    rc = 1
elif command == "print":
    if label:
        if service not in state["loaded"]:
            rc = 1
        else:
            print("    pid = 1234")
elif command == "print-disabled":
    if state.get("disabled_error"):
        rc = 1
    else:
        print('disabled services = {')
        for name in state.get("disabled", []):
            print('    "' + name + '" => true')
        print('}')
elif command == "bootstrap":
    label = Path(args[2]).name.removesuffix(".plist")
    service = uid + "/" + label
    if state.get("logout_on_bootstrap"):
        state["domains"].remove(uid)
        rc = 1
    elif state.get("failures", 0):
        state["failures"] -= 1
        rc = 1
    elif label in state.get("disabled", []) or service in state["loaded"]:
        rc = 1
    else:
        state["loaded"].append(service)
else:
    raise RuntimeError("Unexpected launchctl command: " + repr(args))
path.write_text(json.dumps(state))
sys.exit(rc)
'''


def shell_function(script, name):
    return re.search(rf"^{name}\(\) \{{\n.*?^\}}\n", script, re.MULTILINE | re.DOTALL)[0]


@unittest.skipIf(sys.platform == "win32", "macOS package hooks require a POSIX shell")
class TestMacOSGUILifecycle(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.state_path = self.root / "state.json"
        self.state = {"domains": ["501", "502"], "loaded": [], "calls": []}
        self.post = (HOOKS / "postinst").read_text()
        self.functions = shell_function(self.post, "restore_gui_launchagent")
        self.restore_users = self.post[
            self.post.index("# Start helpers for logged-in users") : self.post.index(
                '\necho ""', self.post.index("# Start helpers for logged-in users")
            )
        ]
        fake = self.root / "launchctl"
        fake.write_text(f"#!{sys.executable}\n" + LAUNCHCTL)
        fake.chmod(0o755)
        self.env = {
            "PATH": f"{self.root}:/usr/bin:/bin",
            "LAUNCHD_STATE": str(self.state_path),
            "AI_USAGE_DESKTOP_MONITOR_LABEL": "com.datadoghq.ai-usage-agent.desktop-monitor",
            "OLD_AI_USAGE_DESKTOP_MONITOR_LABEL": "com.datadoghq.ai-prompt-logger.desktop-monitor",
        }

    def run_shell(self, body, users="alice console\nalice ttys001\nbob console\nsshuser ttys002\nunknown ttys003"):
        self.state_path.write_text(json.dumps(self.state))
        script = (
            self.functions
            + f"""
who() {{ printf '%s\\n' {shlex.quote(users)}; }}
id() {{
    case "$2" in
        alice) echo 501 ;;
        bob) echo 502 ;;
        sshuser) echo 503 ;;
        *) return 1 ;;
    esac
}}
sleep() {{ :; }}
{body}
"""
        )
        result = subprocess.run(["/bin/sh", "-c", script], env=self.env, capture_output=True, text=True, timeout=60)
        self.state = json.loads(self.state_path.read_text())
        self.assertEqual(result.stderr, "")
        return result

    def restore(self, uid="501"):
        return self.run_shell(f'restore_gui_launchagent {uid} {LABEL} /Library/LaunchAgents/{LABEL}.plist')

    def bootstraps(self):
        return [call for call in self.state["calls"] if call[0] == "bootstrap"]

    def test_logged_in_gui_users_are_restored_once(self):
        result = self.run_shell(self.restore_users)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(set(self.state["loaded"]), {f"501/{LABEL}", f"502/{LABEL}"})
        self.assertEqual(len(self.bootstraps()), 2)

    def test_desktop_monitor_uses_the_same_gui_sessions(self):
        template = self.root / "desktop-monitor.plist.example"
        template.touch()
        self.env["AI_USAGE_DESKTOP_MONITOR_PLIST_EXAMPLE"] = str(template)
        self.env["AI_USAGE_DESKTOP_MONITOR_PLIST"] = (
            "/Library/LaunchAgents/com.datadoghq.ai-usage-agent.desktop-monitor.plist"
        )
        result = self.run_shell(self.restore_users)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(len(self.bootstraps()), 4)
        self.assertEqual({call[1] for call in self.bootstraps()}, {"gui/501", "gui/502"})

    def test_already_loaded_helper_is_not_bootstrapped_again(self):
        self.state["loaded"] = [f"501/{LABEL}"]
        self.assertEqual(self.restore().returncode, 0)
        self.assertEqual(self.bootstraps(), [])

    def test_explicit_disable_is_preserved(self):
        self.state["disabled"] = [LABEL]
        result = self.restore()
        self.assertEqual(result.returncode, 0)
        self.assertIn("Preserving disabled", result.stdout)
        self.assertEqual(self.bootstraps(), [])

    def test_disabled_state_query_failure_is_reported(self):
        self.state["disabled_error"] = True
        result = self.restore()
        self.assertEqual(result.returncode, 1)
        self.assertIn("Could not read disabled state", result.stdout)
        self.assertEqual(self.bootstraps(), [])

    def test_bootstrap_transient_failure_retries(self):
        self.state["failures"] = 2
        self.assertEqual(self.restore().returncode, 0)
        self.assertEqual(len(self.bootstraps()), 3)

    def test_bootstrap_failure_is_bounded(self):
        self.state["failures"] = 100
        result = self.restore()
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(self.bootstraps()), 10)
        self.assertIn("did not start", result.stdout)

    def test_logout_during_bootstrap_is_not_an_install_failure(self):
        self.state["logout_on_bootstrap"] = True
        result = self.restore()
        self.assertEqual(result.returncode, 0)
        self.assertIn("ended during restoration", result.stdout)
        self.assertEqual(len(self.bootstraps()), 1)

    def test_no_users_at_install_time(self):
        self.state["domains"] = []
        result = self.run_shell(self.restore_users, users="")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(self.bootstraps(), [])
