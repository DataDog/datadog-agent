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
SHELL = os.environ.get('PROCMGR_TEST_SHELL', '/bin/sh')
SYSV_TEMPLATE = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.action.in'
UPSTART_TEMPLATES = (
    REPO_ROOT / 'packages/debian/etc/upstart_debian.action.conf.in',
    REPO_ROOT / 'packages/redhat/etc/upstart_redhat.action.conf.in',
)
ADMIN_PROCMGR_DEFAULTS = '/etc/default/datadog-agent-procmgr'
PAR_DEFINITIONS = (
    'datadog-agent-par-control.yaml',
    'datadog-agent-action-executor.yaml',
)
INSTALLER_OPT_OUT = 'DATADOG_ENABLED=no\n'
SOCKET = '/var/run/datadog-procmgrd/dd-procmgrd.sock'
UNIX_HEADER = 'Num       RefCount Protocol Flags    Type St Inode Path\n'
LISTENING = f'0000000000000000: 00000002 00000000 00010000 0001 01 12345 {SOCKET}\n'
# Same path, but a connected socket: not a listener.
CONNECTED = f'0000000000000000: 00000002 00000000 00000000 0001 03 12345 {SOCKET}\n'
# Each case disables procmgr, so without a listening socket the action job forces the monolithic runner.
# Only the admin opt-out keeps both PAR definitions, so only it lets a listening socket defer to the config.
PROCMGR_DISABLED = (
    ('not installed', {'procmgr_installed': False}),
    ('installer opt-out', {'procmgr_installed': True, 'installer_defaults': INSTALLER_OPT_OUT}),
    ('admin opt-out', {'procmgr_installed': True, 'admin_defaults': 'DATADOG_ENABLED=no\n'}),
)
LISTENING_DEFERS_TO_CONFIG = {'admin opt-out'}
ADMIN_ENABLE = 'DATADOG_ENABLED=yes\nDD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED=leaked\n'
# With the installer opt-out no PAR definition is written, so these files are the whole catalog.
MISSING_PAR_DEFINITIONS = (
    ('empty catalog', ()),
    ('par-control only', ('datadog-agent-par-control.yaml',)),
    ('action-executor only', ('datadog-agent-action-executor.yaml',)),
    ('custom definition only', ('custom.yaml',)),
)
PAR_DEFINITION_SETS = (
    ('yaml', PAR_DEFINITIONS),
    ('yml', ('datadog-agent-par-control.yml', 'datadog-agent-action-executor.yml')),
)
SOCKETS = (('absent', None), ('listening', LISTENING))


@unittest.skipUnless(os.name == 'posix', 'init template tests require POSIX')
class TestPARLegacyStartupTemplates(unittest.TestCase):
    def executable(self, path, content):
        path.write_text('#!/bin/sh\nset -eu\n' + content)
        path.chmod(0o755)

    def write_procmgr_files(self, root, procmgr_installed, installer_defaults, admin_defaults, extra_processes=()):
        if procmgr_installed:
            self.executable(root / 'embedded/bin/dd-procmgrd', 'exit 0\n')
        # installer_defaults opt-out means the installer wrote no Private Action Runner definitions.
        names = ()
        if procmgr_installed and installer_defaults != INSTALLER_OPT_OUT:
            names = PAR_DEFINITIONS
        names = tuple(names) + tuple(extra_processes)
        if names:
            directory = root / 'processes.d'
            directory.mkdir(exist_ok=True)
            for name in names:
                (directory / name).write_text('command: /bin/true\n')
        admin = root / 'default/datadog-agent-procmgr'
        if admin_defaults is not None:
            admin.parent.mkdir(exist_ok=True)
            admin.write_text(admin_defaults)
        return admin

    def run_sysv(
        self,
        action='start',
        statuses='',
        running=False,
        setpriv=True,
        unix_row=None,
        procmgr_installed=False,
        installer_defaults=None,
        admin_defaults=None,
        extra_processes=(),
        **env,
    ):
        import fcntl

        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'embedded/bin/privateactionrunner'
            binary.parent.mkdir(parents=True)
            self.executable(binary, 'exit 0\n')
            self.write_procmgr_files(root, procmgr_installed, installer_defaults, admin_defaults, extra_processes)
            (root / 'run').mkdir()
            pidfile = root / 'run/privateactionrunner.pid'
            if running:
                pidfile.write_text(f'{os.getpid()}\n')
            commands = root / 'commands'
            commands.mkdir()
            (commands / 'awk').symlink_to(shutil.which('awk'))
            (commands / 'cat').symlink_to(shutil.which('cat'))
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
                '''echo "$* split=${DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED:-unset}" >> "$CALLS"
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
            unix_table = root / 'unix'
            unix_table.write_text(UNIX_HEADER + (unix_row or ''))
            script = script.replace('/proc/net/unix', str(unix_table))
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
                    [SHELL, str(rendered), action],
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

    def test_start_defers_split_mode_to_config_when_procmgr_is_enabled(self):
        result, calls, sleeps, _ = self.run_sysv(procmgr_installed=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(calls, r'--start .*--make-pidfile .*split=true')
        self.assertNotIn('split=false', calls)
        self.assertIn('split mode from config', result.stdout)
        self.assertNotIn('monolithic', result.stdout)
        self.assertEqual(sleeps, ['1', '1'])

    def test_sysv_unrelated_definition_does_not_select_split_mode(self):
        result, calls, _, _ = self.run_sysv(
            procmgr_installed=True, installer_defaults=INSTALLER_OPT_OUT, extra_processes=('custom.yaml',)
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
        self.assertIn('monolithic mode', result.stdout)

    def test_sysv_forces_monolithic_when_procmgr_is_disabled(self):
        for name, procmgr in PROCMGR_DISABLED:
            with self.subTest(name):
                result, calls, sleeps, _ = self.run_sysv(**procmgr)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
                self.assertIn('monolithic mode', result.stdout)
                self.assertEqual(sleeps, ['1', '1'])

    def test_sysv_listening_procmgr_defers_to_config_only_with_par_definitions(self):
        for name, procmgr in PROCMGR_DISABLED:
            with self.subTest(name):
                result, calls, sleeps, _ = self.run_sysv(unix_row=LISTENING, **procmgr)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                if name in LISTENING_DEFERS_TO_CONFIG:
                    self.assertRegex(calls, r'--start .*--make-pidfile .*split=true')
                    self.assertNotIn('split=false', calls)
                    self.assertIn('split mode from config', result.stdout)
                else:
                    self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
                    self.assertIn('monolithic mode', result.stdout)
                self.assertEqual(sleeps, ['1', '1'])

    def test_sysv_ignores_a_socket_that_is_not_listening(self):
        result, calls, _, _ = self.run_sysv(unix_row=CONNECTED)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
        self.assertIn('monolithic mode', result.stdout)

    def test_sysv_admin_enable_without_par_definitions_stays_monolithic(self):
        for catalog, processes in MISSING_PAR_DEFINITIONS:
            for socket, row in SOCKETS:
                with self.subTest(catalog=catalog, socket=socket):
                    result, calls, sleeps, _ = self.run_sysv(
                        unix_row=row,
                        procmgr_installed=True,
                        installer_defaults=INSTALLER_OPT_OUT,
                        admin_defaults=ADMIN_ENABLE,
                        extra_processes=processes,
                    )
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertRegex(calls, r'--start .*--make-pidfile .*split=false')
                    self.assertIn('monolithic mode', result.stdout)
                    self.assertNotIn('leaked', calls)
                    self.assertEqual(sleeps, ['1', '1'])

    def test_sysv_admin_enable_with_par_definitions_defers_to_config(self):
        for suffix, processes in PAR_DEFINITION_SETS:
            with self.subTest(suffix):
                result, calls, sleeps, _ = self.run_sysv(
                    procmgr_installed=True,
                    installer_defaults=INSTALLER_OPT_OUT,
                    admin_defaults=ADMIN_ENABLE,
                    extra_processes=processes,
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertRegex(calls, r'--start .*--make-pidfile .*split=true')
                self.assertIn('split mode from config', result.stdout)
                self.assertNotIn('leaked', calls)
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
            # Keep a host dd-procmgrd socket from switching this test to split mode.
            unix_table = root / 'unix'
            unix_table.write_text(UNIX_HEADER)
            script = script.replace('/proc/net/unix', str(unix_table))
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
                return subprocess.run([SHELL, str(rendered), action], text=True, capture_output=True, timeout=30)

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

    def run_upstart(
        self,
        template,
        unix_row=None,
        procmgr_installed=True,
        installer_defaults=None,
        admin_defaults=None,
        extra_processes=(),
    ):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binary = root / 'embedded/bin/privateactionrunner'
            binary.parent.mkdir(parents=True)
            self.executable(binary, 'echo "split=${DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED-}"\n')
            admin = self.write_procmgr_files(
                root, procmgr_installed, installer_defaults, admin_defaults, extra_processes
            )
            # Upstart's script/end script delimiters are syntax, not layout comments.
            text = template.read_text()
            blocks = re.findall(r'^script\n(.*?)^end script$', text, re.MULTILINE | re.DOTALL)
            self.assertEqual(len(blocks), 1, f'{template}: expected one main script block')
            script = blocks[0].replace('{install_dir}', tmp).replace('{etc_dir}', tmp)
            logfile = root / 'errors.log'
            unix_table = root / 'unix'
            unix_table.write_text(UNIX_HEADER + (unix_row or ''))
            script = script.replace('/proc/net/unix', str(unix_table))
            script = script.replace(ADMIN_PROCMGR_DEFAULTS, str(admin))
            script = script.replace('&>>', '>>')
            script = script.replace('/var/log/datadog/privateactionrunner-errors.log', str(logfile))
            su_env = root / 'su.env'
            # Record the environment su receives, then clear it. Real su does not pass it on.
            self.executable(
                root / 'su',
                '''env > "$SU_ENV"
shell=$2
command=$4
shift 6
exec env -i PATH="$PATH" "$shell" -c "$command" "$@"
''',
            )
            self.executable(
                root / 'start-stop-daemon',
                '''case " $* " in *" --background "*) exit 96 ;; esac
case " $* " in *" --chuid dd-agent "*) ;; *) exit 97 ;; esac
case " $* " in *" --pidfile /nonexistent "*) ;; *) exit 95 ;; esac
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
            # Upstart runs script blocks with sh -e. Debian's /bin/sh is dash.
            result = subprocess.run(
                [SHELL, '-e', '-c', script],
                env={
                    **os.environ,
                    'PATH': f'{tmp}:/usr/bin:/bin',
                    'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': 'true',
                    'SU_ENV': str(su_env),
                },
                text=True,
                capture_output=True,
                check=False,
            )
            output = result.stdout + (logfile.read_text() if logfile.exists() else '')
            received = su_env.read_text() if su_env.exists() else ''
            return result, output, received

    def test_upstart_script_forces_monolithic_only_without_procmgr(self):
        for template in UPSTART_TEMPLATES:
            text = template.read_text()
            self.assertIn(SOCKET, text)
            self.assertNotIn('sleep', text.split('\nscript\n', 1)[1].split('end script', 1)[0])
            self.assertNotIn('initctl', text)
            if template.name.startswith('upstart_debian'):
                self.assertNotIn('\nsetuid ', f'\n{text}')
                command = text.split('exec start-stop-daemon', 1)[1].split('end script', 1)[0]
                self.assertIn('--chuid dd-agent', command)
                self.assertIn('--pidfile /nonexistent', command)
                self.assertNotIn('--background', command)
            with self.subTest(template=template.name, procmgr='enabled'):
                result, output, _ = self.run_upstart(template)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('split mode from config', output)
                self.assertNotIn('split=false', output)
                # Red Hat su clears the environment, so an inherited true does not reach the runner.
                if template.name.startswith('upstart_debian'):
                    self.assertIn('split=true', output)
            for name, procmgr in PROCMGR_DISABLED:
                for socket, row in (('absent', None), ('connected', CONNECTED)):
                    with self.subTest(template=template.name, procmgr=name, socket=socket):
                        result, output, _ = self.run_upstart(template, row, **procmgr)
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertIn('monolithic mode', output)
                        self.assertIn('split=false', output)
                        self.assertNotIn('not supported', output)
                with self.subTest(template=template.name, procmgr=name, socket='listening'):
                    result, output, _ = self.run_upstart(template, LISTENING, **procmgr)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    if name in LISTENING_DEFERS_TO_CONFIG:
                        self.assertIn('split mode from config', output)
                        self.assertNotIn('split=false', output)
                    else:
                        self.assertIn('monolithic mode', output)
                        self.assertIn('split=false', output)

    def test_upstart_unrelated_definition_does_not_select_split_mode(self):
        for template in UPSTART_TEMPLATES:
            with self.subTest(template=template.name):
                result, output, _ = self.run_upstart(
                    template,
                    procmgr_installed=True,
                    installer_defaults=INSTALLER_OPT_OUT,
                    extra_processes=('custom.yaml',),
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertIn('monolithic mode', output)
                self.assertIn('split=false', output)

    def test_upstart_admin_enable_without_par_definitions_stays_monolithic(self):
        for template in UPSTART_TEMPLATES:
            for catalog, processes in MISSING_PAR_DEFINITIONS:
                for socket, row in SOCKETS:
                    with self.subTest(template=template.name, catalog=catalog, socket=socket):
                        result, output, su_env = self.run_upstart(
                            template,
                            row,
                            installer_defaults=INSTALLER_OPT_OUT,
                            admin_defaults=ADMIN_ENABLE,
                            extra_processes=processes,
                        )
                        self.assertEqual(result.returncode, 0, result.stderr)
                        self.assertIn('monolithic mode', output)
                        # On Red Hat this comes from the runner, so the flag made it through su -c.
                        self.assertIn('split=false', output)
                        self.assertNotIn('leaked', output)
                        self.assertNotIn('leaked', su_env)

    def test_upstart_admin_enable_with_par_definitions_defers_to_config(self):
        for template in UPSTART_TEMPLATES:
            for suffix, processes in PAR_DEFINITION_SETS:
                with self.subTest(template=template.name, definitions=suffix):
                    result, output, su_env = self.run_upstart(
                        template,
                        installer_defaults=INSTALLER_OPT_OUT,
                        admin_defaults=ADMIN_ENABLE,
                        extra_processes=processes,
                    )
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertIn('split mode from config', output)
                    self.assertNotIn('split=false', output)
                    self.assertNotIn('leaked', output)
                    # su clears the environment before the runner, so the runner output cannot show this leak.
                    if template.name.startswith('upstart_redhat'):
                        self.assertIn('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED=true', su_env)
                        self.assertNotIn('leaked', su_env)
