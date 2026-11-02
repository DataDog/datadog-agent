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
elif command == "bootout":
    if not state.get("stuck") and service in state["loaded"]:
        state["loaded"].remove(service)
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
        self.snapshot = self.root / "gui-session-uids"
        self.snapshot.write_text("")
        self.state = {"domains": ["501", "502"], "loaded": [], "calls": []}
        self.pre = (HOOKS / "preinst").read_text()
        self.post = (HOOKS / "postinst").read_text()
        self.functions = "\n".join(
            shell_function(self.post, name)
            for name in ("gui_session_uids", "installation_gui_session_uids", "restore_gui_launchagent")
        )
        fake = self.root / "launchctl"
        fake.write_text(f"#!{sys.executable}\n" + LAUNCHCTL)
        fake.chmod(0o755)
        self.env = {
            "PATH": f"{self.root}:/usr/bin:/bin",
            "LAUNCHD_STATE": str(self.state_path),
            "GUI_SESSION_FILE": str(self.snapshot),
            "AI_USAGE_DESKTOP_MONITOR_LABEL": "com.datadoghq.ai-usage-agent.desktop-monitor",
            "OLD_AI_USAGE_DESKTOP_MONITOR_LABEL": "com.datadoghq.ai-prompt-logger.desktop-monitor",
        }

    def run_shell(self, body, process_uids="0 450 501 502 502 503"):
        self.state_path.write_text(json.dumps(self.state))
        script = (
            self.functions
            + f"""
ps() {{ printf '%s\\n' {shlex.quote(process_uids)} | tr ' ' '\\n'; }}
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

    def test_discovery_uses_live_domains_and_deduplicates(self):
        result = self.run_shell("gui_session_uids")
        self.assertEqual(result.stdout, "501\n502\n")
        self.assertEqual(result.returncode, 0)

    def test_discovery_is_identical_in_both_hooks(self):
        self.assertEqual(shell_function(self.pre, "gui_session_uids"), shell_function(self.post, "gui_session_uids"))
        self.assertLess(
            self.pre.index('gui_session_uids > "$GUI_SESSION_FILE"'),
            self.pre.index("\ncleanup_per_user_installations\n"),
        )

    def test_discovery_failure_is_reported(self):
        result = self.run_shell("ps() { return 1; }; gui_session_uids 2>&1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Could not discover", result.stdout)

    def test_snapshot_and_new_sessions_are_reconciled(self):
        # 501 was stopped, 502 logged out, 503 is a new login. 504's missing helper
        # predates the upgrade. Invalid state must never become a launchctl target.
        self.snapshot.write_text("501\n502\n$(touch injected)\n0\n")
        self.state["domains"] = ["501", "503", "504"]
        result = self.run_shell(
            f'for uid in $(installation_gui_session_uids); do restore_gui_launchagent "$uid" {LABEL} /Library/LaunchAgents/{LABEL}.plist; done',
            process_uids="0 503 504",
        )
        self.assertEqual(result.returncode, 0)
        self.assertEqual(set(self.state["loaded"]), {f"{uid}/{LABEL}" for uid in ("501", "503", "504")})
        self.assertIn("session 502 ended", result.stdout)

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
        result = self.run_shell("installation_gui_session_uids")
        self.assertEqual(result.stdout, "")
        self.assertEqual(self.bootstraps(), [])

    def test_stuck_old_registration_is_not_reported_as_restored(self):
        self.state["loaded"] = [f"501/{LABEL}"]
        self.state["stuck"] = True
        stop = self.post[self.post.index('GUI_STOP_FAILED=""') : self.post.index("# Wait for GUI processes")]
        result = self.run_shell(stop + f'\nrestore_gui_launchagent 501 {LABEL} /Library/LaunchAgents/{LABEL}.plist')
        self.assertEqual(result.returncode, 1)
        self.assertIn("old registration did not stop", result.stdout)
        self.assertEqual(self.bootstraps(), [])
