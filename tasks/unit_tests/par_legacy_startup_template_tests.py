"""Regression tests for the PAR init templates in packages/{debian,redhat}/etc.

Kept in the invoke unit-test suite so the existing tooling CI runs them. Execute
whole rendered SysV scripts (including action dispatch), not slices of functions.
"""

import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SYSV_TEMPLATE = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.action.in'
UPSTART_TEMPLATES = (
    REPO_ROOT / 'packages/debian/etc/upstart_debian.action.conf.in',
    REPO_ROOT / 'packages/redhat/etc/upstart_redhat.action.conf.in',
)


@unittest.skipUnless(os.name == 'posix', 'init template tests require POSIX')
class TestPARLegacyStartupTemplates(unittest.TestCase):
    def executable(self, path, content):
        path.write_text('#!/bin/sh\nset -eu\n' + content)
        path.chmod(0o755)

    def run_sysv(self, action='start', statuses='', running=False, setpriv=True, **env):
        import fcntl

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'embedded/bin/privateactionrunner'
            binary.parent.mkdir(parents=True)
            self.executable(binary, 'exit 0\n')
            (root / 'run').mkdir()
            pidfile = root / 'run/privateactionrunner.pid'
            if running:
                pidfile.write_text(f'{os.getpid()}\n')
            commands = root / 'commands'
            commands.mkdir()
            self.executable(
                commands / 'rm',
                f'[ "${{REMOVE_ERROR:-0}}" = 0 ] || exit "$REMOVE_ERROR"\nexec "{shutil.which("rm")}" "$@"\n',
            )
            self.executable(commands / 'id', 'echo 1234\n')
            # Exercise real child exits and session detachment, including on macOS.
            session = root / 'setsid.py'
            session.write_text('''import os, sys
if os.getpgrp() == os.getpid() and os.fork():
    sys.exit(0)
os.setsid()
os.execv(sys.argv[1], sys.argv[1:])
''')
            self.executable(commands / 'setsid', f'exec "{sys.executable}" "{session}" "$@"\n')
            runner = root / 'runner.py'
            runner.write_text('''import os, signal, sys
from pathlib import Path
assert os.getpid() == os.getpgrp() == os.getsid(0)
try:
    os.fstat(int(os.environ['INHERITED_FD']))
except OSError:
    pass
else:
    raise AssertionError('descriptor leaked into the daemon')
signal.signal(signal.SIGUSR1, lambda *_: sys.exit(int(os.environ['RUNNER_EXIT'])))
if os.environ['SPAWN_ERROR'] == '0':
    Path(os.environ['PIDFILE']).write_text(str(os.getpid()) + '\\n')
Path(os.environ['CHILD_PID']).write_text(str(os.getpid()))
if os.environ['SPAWN_ERROR'] != '0':
    sys.exit(int(os.environ['SPAWN_ERROR']))
while True:
    signal.pause()
''')
            sleeper = root / 'sleep.py'
            sleeper.write_text('''import os, signal, time
from pathlib import Path
sleeps = Path(os.environ['SLEEPS'])
with sleeps.open('a') as stream:
    stream.write('1\\n')
deadline = time.monotonic() + 5
child = Path(os.environ['CHILD_PID'])
while not child.exists():
    assert time.monotonic() < deadline, 'launcher did not spawn a child'
    time.sleep(0.005)
pid = int(child.read_text())
should_exit = os.environ['RUNNER_EXIT'] != 'running' and len(sleeps.read_text().splitlines()) > int(os.environ['EXIT_AFTER'])
if should_exit and os.environ['SPAWN_ERROR'] == '0':
    os.kill(pid, signal.SIGKILL if os.environ['RUNNER_EXIT'] == 'signal' else signal.SIGUSR1)
if should_exit or os.environ['SPAWN_ERROR'] != '0':
    while True:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            break
        assert time.monotonic() < deadline, 'launcher did not reap its child'
        time.sleep(0.005)
''')
            self.executable(commands / 'sleep', f'exec "{sys.executable}" "{sleeper}"\n')
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
                '''case " $* " in *" --exec $PROCMGR "*) echo "$*" >> "$PROCMGR_CALLS"; exit "$PROCMGR_STATUS" ;; esac
echo "$* split=${DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED:-unset}" >> "$CALLS"
case " $* " in *" --exec "*) ;; *) exit 90 ;; esac
case "$1" in
    --status)
        [ "${AS_AGENT:-}" = yes ] || exit 91
        [ "$CHECK_ERROR" = 0 ] || exit "$CHECK_ERROR"
        [ -f "$PIDFILE" ] || exit 3
        if [ -z "$STATUSES" ]; then
            read -r pid < "$PIDFILE"
            kill -0 "$pid" 2>/dev/null && exit 0 || exit 1
        fi
        count=0
        if [ -f "$COUNT" ]; then read -r count < "$COUNT"; fi
        echo "$((count + 1))" > "$COUNT"
        set -- $STATUSES
        while [ "$count" -gt 0 ] && [ "$#" -gt 1 ]; do shift; count=$((count - 1)); done
        exit "$1"
        ;;
    --start)
        case " $* " in *" --make-pidfile "*) ;; *) exit 92 ;; esac
        case " $* " in
            *" --background "*) "$PYTHON" "$SESSION" "$PYTHON" "$RUNNER" >/dev/null 2>&1 & ;;
            *) exec "$PYTHON" "$RUNNER" ;;
        esac
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
            child_pid = root / 'child-pid'
            with (root / 'inherited-fd').open('w+b') as stream:
                inherited_fd = fcntl.fcntl(stream.fileno(), fcntl.F_DUPFD, 20)
            environment = {
                **os.environ,
                'PATH': str(commands),
                'POSIXLY_CORRECT': '1',
                'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': 'true',
                'PIDFILE': str(pidfile),
                'CALLS': str(calls),
                'PROCMGR': str(root / 'embedded/bin/dd-procmgrd'),
                'PROCMGR_CALLS': str(root / 'procmgr-calls'),
                'PROCMGR_STATUS': '1',
                'SLEEPS': str(sleeps),
                'COUNT': str(root / 'count'),
                'STATUSES': statuses,
                'CHECK_ERROR': '0',
                'SPAWN_ERROR': '0',
                'STOP_ERROR': '0',
                'REMOVE_ERROR': '0',
                'PYTHON': sys.executable,
                'RUNNER': str(runner),
                'SESSION': str(session),
                'CHILD_PID': str(child_pid),
                'INHERITED_FD': str(inherited_fd),
                'RUNNER_EXIT': 'running',
                'EXIT_AFTER': '0',
                **env,
            }
            try:
                result = subprocess.run(
                    ['/bin/sh', str(rendered), action],
                    env=environment,
                    pass_fds=(inherited_fd,),
                    text=True,
                    capture_output=True,
                    check=False,
                    timeout=15,
                )
                return result, calls.read_text(), sleeps.read_text().splitlines(), pidfile.exists()
            finally:
                os.close(inherited_fd)
                if child_pid.exists():
                    try:
                        os.kill(int(child_pid.read_text()), signal.SIGKILL)
                    except ProcessLookupError:
                        pass

    def test_start_checks_two_live_samples_as_agent_and_forces_monolithic(self):
        for setpriv in (True, False):
            with self.subTest(setpriv=setpriv):
                result, calls, sleeps, _ = self.run_sysv(setpriv=setpriv)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(sleeps, ['1', '1'])
                self.assertEqual(calls.count('--status'), 3)
                self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
                self.assertIn('monolithic mode', result.stdout)
                self.assertNotIn('WARNING', result.stdout)

    def test_start_defers_split_mode_to_config_when_procmgr_runs(self):
        result, calls, sleeps, _ = self.run_sysv(PROCMGR_STATUS='0')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(calls, r'--start .*--make-pidfile .*split=true')
        self.assertNotIn('split=false', calls)
        self.assertIn('split mode from config', result.stdout)
        self.assertNotIn('monolithic', result.stdout)
        self.assertEqual(sleeps, ['1', '1'])

    def test_start_accepts_clean_exit_and_removes_pidfile(self):
        for setpriv in (True, False):
            for after in ('0', '1'):
                with self.subTest(setpriv=setpriv, after=after):
                    result, _, sleeps, pid_exists = self.run_sysv(setpriv=setpriv, RUNNER_EXIT='0', EXIT_AFTER=after)
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertIn('exited cleanly', result.stdout)
                    self.assertNotIn('result=1', result.stdout)
                    self.assertFalse(pid_exists)
                    self.assertEqual(len(sleeps), int(after) + 1)

    def test_clean_exit_does_not_hide_pid_cleanup_failure(self):
        result, _, _, pid_exists = self.run_sysv(RUNNER_EXIT='0', REMOVE_ERROR='2')
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertTrue(pid_exists)
        self.assertNotIn('exited cleanly', result.stdout)

    def test_start_reports_nonzero_and_signal_exits(self):
        for outcome in ('1', '2', 'signal'):
            for after in ('0', '1'):
                with self.subTest(outcome=outcome, after=after):
                    result, _, sleeps, _ = self.run_sysv(RUNNER_EXIT=outcome, EXIT_AFTER=after)
                    self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
                    self.assertNotIn('exited cleanly', result.stdout)
                    self.assertEqual(len(sleeps), int(after) + 1)

    def test_start_fails_if_live_child_never_matches(self):
        result, _, sleeps, _ = self.run_sysv(statuses='1')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(sleeps), 5)

    def test_start_fails_on_status_error(self):
        result, _, sleeps, _ = self.run_sysv(statuses='4')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(sleeps, ['1'])

    def test_start_allows_delayed_pidfile(self):
        result, _, sleeps, _ = self.run_sysv(statuses='3 0 0')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(sleeps), 3)

    def test_start_times_out_without_pidfile(self):
        result, _, sleeps, _ = self.run_sysv(statuses='3')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(len(sleeps), 5)

    def test_already_running_does_not_spawn_or_wait(self):
        result, calls, sleeps, _ = self.run_sysv(running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('--start', calls)
        self.assertEqual(sleeps, [])

    def test_start_check_error_is_not_already_running(self):
        result, calls, sleeps, _ = self.run_sysv(CHECK_ERROR='4')
        self.assertEqual(result.returncode, 1)
        self.assertNotIn('--start', calls)
        self.assertEqual(sleeps, [])

    def test_failed_spawn_without_pidfile_is_not_a_clean_exit(self):
        result, _, sleeps, pid_exists = self.run_sysv(SPAWN_ERROR='2')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(sleeps, ['1'])
        self.assertFalse(pid_exists)
        self.assertNotIn('exited cleanly', result.stdout)

    def test_status_uses_same_agent_executable_check(self):
        for status in (0, 1, 3, 4):
            with self.subTest(status=status):
                result, calls, sleeps, _ = self.run_sysv('status', statuses=str(status), running=True)
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertEqual(calls.count('--status'), 1)
                self.assertIn(
                    'is running' if status == 0 else 'not running' if status in (1, 3) else 'Could not', result.stdout
                )
                self.assertEqual(sleeps, [])

    def test_stop_checks_executable_as_agent(self):
        for status in (0, 1, 2):
            with self.subTest(status=status):
                result, calls, _, _ = self.run_sysv('stop', running=True, STOP_ERROR=str(status))
                self.assertEqual(result.returncode, 0 if status in (0, 1) else 1, result.stderr)
                self.assertIn('--stop --quiet --retry=30', calls)

    def test_restart_dispatch_stops_then_starts(self):
        result, calls, sleeps, _ = self.run_sysv('restart', running=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertLess(calls.index('--stop'), calls.index('--start'))
        self.assertEqual(sleeps, ['1', '1'])

    @unittest.skipUnless(
        sys.platform == 'linux'
        and os.geteuid() == 0
        and Path('/sbin/start-stop-daemon').exists()
        and shutil.which('setsid'),
        'native SysV utility test requires Linux, root, start-stop-daemon, and setsid',
    )
    def test_native_sysv_utilities_preserve_exit_status(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'embedded/bin/privateactionrunner'
            binary.parent.mkdir(parents=True)
            binary.symlink_to(Path(sys.executable).resolve())
            (root / 'run').mkdir()
            pidfile = root / 'run/privateactionrunner.pid'
            runner = root / 'runner.py'
            runner.write_text('''import os, signal, sys
assert os.getsid(0) == os.getpgrp() == os.getpid()
assert os.environ['DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'] == 'false'
if sys.argv[1] == 'running':
    while True:
        signal.pause()
if sys.argv[1] == 'signal':
    os.kill(os.getpid(), signal.SIGKILL)
sys.exit(int(sys.argv[1]))
''')
            script = SYSV_TEMPLATE.read_text().replace('{install_dir}', tmp).replace('{etc_dir}', tmp)
            script = script.replace('AGENT_USER="dd-agent"', 'AGENT_USER="root"')
            script = script.replace('/etc/default/', f'{tmp}/default/')
            script = script.replace(
                '. /lib/lsb/init-functions',
                '''init_is_upstart() { return 1; }
log_daemon_msg() { echo "$*"; }
log_end_msg() { return "$1"; }
log_success_msg() { echo "$*"; }
log_failure_msg() { echo "$*"; }''',
            )
            rendered = root / 'init-script'

            def service(action):
                return subprocess.run(['/bin/sh', str(rendered), action], text=True, capture_output=True, timeout=15)

            for outcome in ('0', '2', 'signal', 'running'):
                with self.subTest(outcome=outcome):
                    rendered.write_text(
                        script.replace(
                            'AGENT_ARGS="run --cfgpath=$ETC_DIR/datadog.yaml"', f'AGENT_ARGS="{runner} {outcome}"'
                        )
                    )
                    try:
                        result = service('start')
                        self.assertEqual(
                            result.returncode, 0 if outcome in ('0', 'running') else 1, result.stdout + result.stderr
                        )
                        status = service('status').returncode
                        self.assertEqual(status, 3 if outcome == '0' else 0 if outcome == 'running' else 1)
                        self.assertEqual(pidfile.exists(), outcome != '0')
                        if outcome in ('0', 'running'):
                            self.assertEqual(service('restart').returncode, 0)
                            self.assertEqual(service('status').returncode, status)
                    finally:
                        service('stop')

    def test_upstart_script_forces_monolithic_only_without_procmgr(self):
        for template in UPSTART_TEMPLATES:
            for state, split, mode in (
                ('stop/waiting', 'false', 'monolithic mode'),
                ('start/running', 'true', 'split mode from config'),
            ):
                with self.subTest(template=template.name, state=state), tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    binary = root / 'embedded/bin/privateactionrunner'
                    binary.parent.mkdir(parents=True)
                    self.executable(binary, 'echo "split=${DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED-}"\n')
                    # Upstart's script/end script delimiters are syntax, not layout comments.
                    text = template.read_text()
                    self.assertIn('initctl status datadog-agent-procmgr', text)
                    self.assertNotIn('initctl --system', text)
                    if template.name.startswith('upstart_debian'):
                        self.assertNotIn('\nsetuid ', f'\n{text}')
                        command = text.split('exec start-stop-daemon', 1)[1].split('end script', 1)[0]
                        self.assertIn('--chuid dd-agent', command)
                        self.assertNotIn('--background', command)
                    blocks = re.findall(r'^script\n(.*?)^end script$', text, re.MULTILINE | re.DOTALL)
                    self.assertEqual(len(blocks), 1, f'{template}: expected one main script block')
                    script = blocks[0].replace('{install_dir}', tmp).replace('{etc_dir}', tmp)
                    logfile = root / 'errors.log'
                    script = script.replace('/var/log/datadog/privateactionrunner-errors.log', str(logfile))
                    self.executable(
                        root / 'su',
                        '''shell=$2
command=$4
shift 6
exec env -i PATH="$PATH" "$shell" -c "$command" "$@"
''',
                    )
                    # Fake initctl reports the procmgr job state; fake sleep keeps the bounded wait instant.
                    self.executable(
                        root / 'initctl',
                        '''[ "$1" = status ] && [ "$2" = datadog-agent-procmgr ] || exit 1
echo "datadog-agent-procmgr $PROCMGR_STATE"
''',
                    )
                    self.executable(
                        root / 'start-stop-daemon',
                        '''case " $* " in *" --background "*) exit 96 ;; esac
case " $* " in *" --chuid dd-agent "*) ;; *) exit 97 ;; esac
exec_path=
while [ $# -gt 0 ]; do
  if [ "$1" = --exec ]; then
    exec_path=$2
    shift 2
    continue
  fi
  if [ "$1" = -- ]; then
    shift
    exec "$exec_path" "$@"
  fi
  shift
done
exit 98
''',
                    )
                    self.executable(root / 'sleep', 'exit 0\n')
                    result = subprocess.run(
                        [shutil.which('bash'), '-c', script],
                        env={
                            **os.environ,
                            'PATH': f'{tmp}:/usr/bin:/bin',
                            'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': 'true',
                            'PROCMGR_STATE': state,
                        },
                        text=True,
                        capture_output=True,
                        check=False,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    output = result.stdout + (logfile.read_text() if logfile.exists() else '')
                    self.assertIn(mode, output)
                    # Red Hat su clears the environment, so an inherited true does not reach the runner.
                    if template.name.startswith('upstart_redhat') and state == 'start/running':
                        self.assertNotIn('split=false', output)
                    else:
                        self.assertIn(f'split={split}', output)
                    self.assertNotIn('not supported', output)
