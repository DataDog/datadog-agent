"""Regression tests for the PAR init templates in packages/{debian,redhat}/etc.

Kept in the invoke unit-test suite so the existing tooling CI runs them. Execute
whole rendered SysV scripts (including action dispatch), not slices of functions.
"""

import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SYSV_TEMPLATE = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.action.in'
UPSTART_TEMPLATES = (
    REPO_ROOT / 'packages/debian/etc/upstart_debian.action.conf.in',
    REPO_ROOT / 'packages/redhat/etc/upstart_redhat.action.conf.in',
)


class TestPARLegacyStartupTemplates(unittest.TestCase):
    def executable(self, path, content):
        path.write_text('#!/bin/sh\nset -eu\n' + content)
        path.chmod(0o755)

    def run_sysv(self, action='start', statuses='0 0', running=False, setpriv=True, **env):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'embedded/bin/privateactionrunner'
            binary.parent.mkdir(parents=True)
            self.executable(binary, 'exit 0\n')
            (root / 'run').mkdir()
            pidfile = root / 'run/privateactionrunner.pid'
            if running:
                pidfile.write_text('1234\n')
            commands = root / 'commands'
            commands.mkdir()
            (commands / 'rm').symlink_to(shutil.which('rm'))
            self.executable(commands / 'id', 'echo 1234\n')
            self.executable(commands / 'sleep', 'echo "$*" >> "$SLEEPS"\n')
            if setpriv:
                self.executable(
                    commands / 'setpriv',
                    '''[ "$1" = --reuid ] && [ "$2" = dd-agent ]
[ "$3" = --regid ] && [ "$4" = 1234 ] && [ "$5" = --init-groups ]
shift 5
AS_AGENT=yes PATH=/usr/bin:/bin exec "$@"
''',
                )
            self.executable(
                commands / 'su',
                '''[ "$1" = -s ] && [ "$3" = -c ] && [ "$5" = -- ] && [ "$6" = dd-agent ]
shell=$2
command=$4
shift 6
AS_AGENT=yes PATH=/usr/bin:/bin exec "$shell" -c "$command" "$@"
''',
            )
            daemon = commands / 'start-stop-daemon'
            self.executable(
                daemon,
                '''echo "$* split=${DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED:-unset}" >> "$CALLS"
case " $* " in *" --exec "*) ;; *) exit 90 ;; esac
case "$1" in
    --status)
        [ "${AS_AGENT:-}" = yes ] || exit 91
        [ "$CHECK_ERROR" = 0 ] || exit "$CHECK_ERROR"
        [ -f "$PIDFILE" ] || exit 3
        count=0
        if [ -f "$COUNT" ]; then read -r count < "$COUNT"; fi
        echo "$((count + 1))" > "$COUNT"
        set -- $STATUSES
        while [ "$count" -gt 0 ] && [ "$#" -gt 1 ]; do shift; count=$((count - 1)); done
        exit "$1"
        ;;
    --start)
        case " $* " in *" --make-pidfile "*) ;; *) exit 92 ;; esac
        [ "$SPAWN_ERROR" = 0 ] || exit "$SPAWN_ERROR"
        echo 1234 > "$PIDFILE"
        ;;
    --stop)
        [ "${AS_AGENT:-}" = yes ] || exit 91
        exit "$STOP_ERROR"
        ;;
    *) exit 93 ;;
esac
''',
            )
            functions = root / 'init-functions'
            functions.write_text('''init_is_upstart() { return 1; }
log_daemon_msg() { echo "$*"; }
log_end_msg() { echo "result=$1"; return "$1"; }
log_success_msg() { echo "$*"; }
log_failure_msg() { echo "$*"; }
log_warning_msg() { echo "WARNING: $*"; }
''')
            script = SYSV_TEMPLATE.read_text().replace('{install_dir}', tmp).replace('{etc_dir}', tmp)
            self.assertEqual(script.count('. /lib/lsb/init-functions'), 1, f'{SYSV_TEMPLATE}: LSB dependency changed')
            script = script.replace('. /lib/lsb/init-functions', f'. "{functions}"')
            script = script.replace('/etc/default/', f'{tmp}/default/')
            script = script.replace('/sbin/start-stop-daemon', str(daemon))
            rendered = root / 'init-script'
            rendered.write_text(script)
            calls, sleeps = root / 'calls', root / 'sleeps'
            calls.touch()
            sleeps.touch()
            result = subprocess.run(
                ['sh', str(rendered), action],
                executable='/bin/sh',
                env={
                    **os.environ,
                    'PATH': str(commands),
                    'POSIXLY_CORRECT': '1',
                    'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': 'true',
                    'PIDFILE': str(pidfile),
                    'CALLS': str(calls),
                    'SLEEPS': str(sleeps),
                    'COUNT': str(root / 'count'),
                    'STATUSES': statuses,
                    'CHECK_ERROR': '0',
                    'SPAWN_ERROR': '0',
                    'STOP_ERROR': '0',
                    **env,
                },
                text=True,
                capture_output=True,
                check=False,
            )
            return result, calls.read_text(), sleeps.read_text().splitlines()

    def test_start_checks_two_live_samples_as_agent_and_forces_monolithic(self):
        for setpriv in (True, False):
            with self.subTest(setpriv=setpriv):
                result, calls, sleeps = self.run_sysv(setpriv=setpriv)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(sleeps, ['1', '1'])
                self.assertEqual(calls.count('--status'), 3)
                self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
                self.assertIn('monolithic mode', result.stdout)
                self.assertNotIn('WARNING', result.stdout)

    def test_start_fails_at_first_dead_sample(self):
        for statuses, expected_sleeps in (('1', 1), ('0 1', 2), ('0 3', 2), ('4', 1)):
            with self.subTest(statuses=statuses):
                result, _, sleeps = self.run_sysv(statuses=statuses)
                self.assertEqual(result.returncode, 1, result.stdout)
                self.assertEqual(len(sleeps), expected_sleeps)

    def test_start_allows_delayed_pidfile(self):
        result, _, sleeps = self.run_sysv(statuses='3 0 0')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(sleeps), 3)

    def test_start_times_out_without_pidfile(self):
        result, _, sleeps = self.run_sysv(statuses='3')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(sleeps), 5)

    def test_already_running_does_not_spawn_or_wait(self):
        result, calls, sleeps = self.run_sysv(running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('--start', calls)
        self.assertEqual(sleeps, [])

    def test_start_check_error_is_not_already_running(self):
        result, calls, sleeps = self.run_sysv(CHECK_ERROR='4')
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('--start', calls)
        self.assertEqual(sleeps, [])

    def test_failed_spawn_does_not_poll(self):
        result, _, sleeps = self.run_sysv(SPAWN_ERROR='2')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(sleeps, [])

    def test_status_uses_same_agent_executable_check(self):
        for status in (0, 1, 3, 4):
            with self.subTest(status=status):
                result, calls, sleeps = self.run_sysv('status', statuses=str(status), running=True)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls.count('--status'), 1)
                self.assertIn(
                    'is running' if status == 0 else 'not running' if status in (1, 3) else 'Could not', result.stdout
                )
                self.assertEqual(sleeps, [])

    def test_stop_checks_executable_as_agent(self):
        for status in (0, 1, 2):
            with self.subTest(status=status):
                result, calls, _ = self.run_sysv('stop', running=True, STOP_ERROR=str(status))
                self.assertEqual(result.returncode, 0 if status in (0, 1) else 1, result.stderr)
                self.assertIn('--stop --quiet --retry=30', calls)

    def test_restart_dispatch_stops_then_starts(self):
        result, calls, sleeps = self.run_sysv('restart', running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(calls.index('--stop'), calls.index('--start'))
        self.assertEqual(sleeps, ['1', '1'])

    def test_upstart_script_blocks_force_monolithic_without_warning(self):
        for template in UPSTART_TEMPLATES:
            with self.subTest(template=template), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                binary = root / 'embedded/bin/privateactionrunner'
                binary.parent.mkdir(parents=True)
                self.executable(binary, 'echo "split=$DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED"\n')
                # Upstart's script/end script delimiters are syntax, not layout comments.
                blocks = re.findall(r'^script\n(.*?)^end script$', template.read_text(), re.MULTILINE | re.DOTALL)
                self.assertEqual(len(blocks), 1, f'{template}: expected one main script block')
                script = blocks[0].replace('{install_dir}', tmp).replace('{etc_dir}', tmp)
                logfile = root / 'errors.log'
                script = script.replace('/var/log/datadog/privateactionrunner-errors.log', str(logfile))
                self.executable(
                    root / 'su',
                    '''shell=$2
command=$4
shift 6
exec "$shell" -c "$command" "$@"
''',
                )
                result = subprocess.run(
                    [shutil.which('bash'), '-c', script],
                    env={
                        **os.environ,
                        'PATH': f'{tmp}:/usr/bin:/bin',
                        'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': 'true',
                    },
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertEqual(result.returncode, 0, result.stderr)
                output = result.stdout + (logfile.read_text() if logfile.exists() else '')
                self.assertIn('split=false', output)
                self.assertIn('monolithic mode', output)
                self.assertNotIn('not supported', output)
