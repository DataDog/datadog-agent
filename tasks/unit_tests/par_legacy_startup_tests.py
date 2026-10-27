import os
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]


class TestPARLegacyStartup(unittest.TestCase):
    def run_sysv_start(self, start_result=0, exits_during_startup=False, check_error=False):
        script = (REPO_ROOT / "packages/debian/etc/sysvinit_debian.action.in").read_text()
        functions = script[script.index("do_start()") : script.index("# Action to take")]
        harness = """
AGENTPATH=/bin/true
AGENT_USER=test
PIDFILE=/unused/privateactionrunner.pid
AGENT_ARGS=run
NAME=datadog-agent-action
DESC='Datadog Private Action Runner'
log_warning_msg() { echo "$*"; }
log_daemon_msg() { echo "$*"; }
log_end_msg() { return "$1"; }
sleep() {
    if [ "$EXITS_DURING_STARTUP" = true ]; then rm -f "$STARTED_FILE"; fi
}
"""
        with tempfile.TemporaryDirectory() as directory:
            daemon = Path(directory) / "start-stop-daemon"
            daemon.write_text('''#!/bin/sh
case " $* " in
    *' --test '*)
        if [ "$CHECK_ERROR" = true ]; then exit 2; fi
        [ ! -f "$STARTED_FILE" ]; exit $? ;;

esac
echo "split=$DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED args=$*"
case " $* " in
    *' --make-pidfile '*) touch "$STARTED_FILE" ;;
esac
exit "$START_RESULT"
''')
            daemon.chmod(0o755)
            su = Path(directory) / "su"
            su.write_text('''#!/bin/sh
while [ "$1" != -- ]; do shift; done
shift
echo 'checking as runner user' >&2
exec "$@"
''')
            su.chmod(0o755)
            return subprocess.run(
                ["sh", "-c", harness + functions + "\nstart_and_wait\n"],
                env={
                    **os.environ,
                    "PATH": directory + os.pathsep + os.environ["PATH"],
                    "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED": "true",
                    "START_RESULT": str(start_result),
                    "CHECK_ERROR": str(check_error).lower(),
                    "EXITS_DURING_STARTUP": str(exits_during_startup).lower(),
                    "STARTED_FILE": str(Path(directory) / "started"),
                },
                capture_output=True,
                text=True,
                timeout=10,
            )

    def test_sysv_starts_monolithic_and_tracks_pid(self):
        result = self.run_sysv_start()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("split=false", result.stdout)
        self.assertIn("--make-pidfile", result.stdout)
        self.assertIn("--exec /bin/true", result.stdout)
        self.assertEqual(result.stderr.count("checking as runner user"), 2)
        self.assertIn("monolithic Private Action Runner", result.stdout)

    def test_sysv_does_not_hide_start_failure(self):
        result = self.run_sysv_start(start_result=1)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertIn("split=false", result.stdout)

    def test_sysv_does_not_treat_check_errors_as_already_running(self):
        result = self.run_sysv_start(check_error=True)
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertNotIn("--background", result.stdout)

    def test_sysv_detects_exit_during_startup(self):
        result = self.run_sysv_start(exits_during_startup=True)
        self.assertEqual(result.returncode, 1, result.stderr)

    def test_upstart_launches_monolithic(self):
        for distro in ("debian", "redhat"):
            with self.subTest(distro=distro):
                template = (REPO_ROOT / f"packages/{distro}/etc/upstart_{distro}.action.conf.in").read_text()
                script = template.split("\nscript\n", 1)[1].split("\nend script", 1)[0]
                # Capture the environment at exec without launching a runner or switching users.
                script = (
                    script[: script.index("  exec ")]
                    + '  printf "split=%s\\n" "$DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"\n'
                )
                result = subprocess.run(
                    ["sh", "-c", script],
                    env={**os.environ, "DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED": "true"},
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("split=false", result.stdout)
                self.assertIn("monolithic Private Action Runner", result.stdout)
