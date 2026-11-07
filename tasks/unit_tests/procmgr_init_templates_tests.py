"""Tests for the dd-procmgrd init templates in packages/{debian,redhat}/etc.

The tests execute the rendered scripts and read the environment the daemon receives. Slicing the
script up to `exec` would pass even when `su` drops the variables before the daemon starts.
"""

import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
SHELL = os.environ.get('PROCMGR_TEST_SHELL', '/bin/sh')
SYSV_PROCMGR = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.procmgr.in'
SYSV_ACTION = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.action.in'
SYSV_AGENT = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.in'
UPSTART_DEBIAN = REPO_ROOT / 'packages/debian/etc/upstart_debian.procmgr.conf.in'
UPSTART_REDHAT = REPO_ROOT / 'packages/redhat/etc/upstart_redhat.procmgr.conf.in'
SYSV_RUNTIME_DIR = '/usr/bin/install -d -o "$AGENT_USER" -g "$AGENT_USER" -m 0755 /var/run/datadog-procmgrd || return 2'
UPSTART_RUNTIME_DIR = '/usr/bin/install -d -o dd-agent -g dd-agent -m 0755 /var/run/datadog-procmgrd || exit 1'
PARENT_SPLIT = 'true'


def render(text, install_dir, etc_dir):
    return text.replace('{install_dir}', str(install_dir)).replace('{etc_dir}', str(etc_dir))


def upstart_block(template, header):
    text = template.read_text() if isinstance(template, Path) else template
    blocks = re.findall(rf'^{header}\n(.*?)^end script$', text, re.MULTILINE | re.DOTALL)
    if len(blocks) != 1:
        raise AssertionError(f'{template}: expected exactly one {header!r} block')
    return blocks[0]


def upstart_main_block(template):
    return upstart_block(template, 'script')


def parse_env(path):
    values = {}
    for line in path.read_text().splitlines():
        key, sep, value = line.partition('=')
        if sep:
            values[key] = value
    return values


def write_executable(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text('#!/bin/sh\n' + content)
    path.chmod(0o755)


def write_agent_commands(commands):
    """Fake setpriv, su and id that mark the start-stop-daemon checks as run by dd-agent."""
    write_executable(commands / 'id', '[ "${ID_RC:-0}" = 0 ] || exit "$ID_RC"\necho 1234\n')
    write_executable(
        commands / 'setpriv',
        '''[ "$1" = --reuid ] && [ "$2" = dd-agent ] && [ "$3" = --regid ] && [ "$4" = 1234 ] && [ "$5" = --init-groups ] || exit 88
shift 5
AS_AGENT=yes exec "$@"
''',
    )
    write_executable(
        commands / 'su',
        '''[ "$1" = -s ] && [ "$3" = -c ] && [ "$5" = -- ] && [ "$6" = dd-agent ] || exit 87
shell=$2
command=$4
shift 6
AS_AGENT=yes exec "$shell" -c "$command" "$@"
''',
    )


class Layout:
    def __init__(self, root):
        self.root = Path(root)
        self.install = self.root / 'install'
        self.etc = self.root / 'etc'
        self.binary = self.install / 'embedded/bin/dd-procmgrd'
        self.env_out = self.root / 'daemon.env'
        self.etc.mkdir(parents=True)
        (self.install / 'run').mkdir(parents=True)
        write_executable(self.binary, f'env > "{self.env_out}"\nexit 0\n')
        definitions = self.install / 'processes.d'
        definitions.mkdir()
        (definitions / 'datadog-agent-par-control.yaml').write_text('command: /bin/true\n')
        # The environment file sets two of the variables that must be overridden by the template.
        (self.etc / 'environment').write_text('DD_EXTRA=from-file\nDD_INSTALL_DIR=/wrong\nDD_CONF_DIR=/wrong\n')

    def expected_four(self):
        return {
            'DD_PM_CONFIG_DIR': f'{self.install}/processes.d',
            'DD_CONF_DIR': str(self.etc),
            'DD_PID_DIR': str(self.install),
            'DD_INSTALL_DIR': str(self.install),
        }


@unittest.skipUnless(os.name == 'posix', 'init template tests require POSIX')
class TestProcmgrInitTemplates(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.root = Path(self._tmp.name)

    def base_env(self, path):
        return {
            **{k: v for k, v in os.environ.items() if not k.startswith('DD_')},
            'PATH': path,
            'DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED': PARENT_SPLIT,
            'DD_PM_CONFIG_DIR': 'from-parent',
            'DD_CONF_DIR': 'from-parent',
            'DD_PID_DIR': 'from-parent',
            'DD_INSTALL_DIR': 'from-parent',
        }

    def test_sysv_start_exports_environment_to_daemon(self):
        source = SYSV_PROCMGR.read_text()
        self.assertNotIn('. "$ETC_DIR/environment"', source)
        self.assertIn('--startas /bin/sh', source)
        self.assertIn('--exec "$AGENTPATH"', source)
        self.assertIn('"/etc/init.d/$NAME" load-env', source)

        layout = Layout(self.root)
        commands = self.root / 'commands'
        started = self.root / 'started'
        argv_path = self.root / 'argv'
        marker = self.root / 'pwned'
        init_functions = self.root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n'
            'log_daemon_msg() { :; }\n'
            'log_end_msg() { return "$1"; }\n'
            'log_success_msg() { :; }\n'
            'log_failure_msg() { :; }\n'
        )
        # --status reports the daemon running once the pid marker exists. The real start
        # executes the child shell, which is where the environment file is applied.
        write_agent_commands(commands)
        write_executable(
            commands / 'start-stop-daemon',
            f'''case " $* " in
  *" --status "*) [ "${{AS_AGENT:-}}" = yes ] || exit 91; [ -f "{started}" ] && exit 0 || exit 3 ;;
esac
case " $* " in
  *" --start "*) [ -z "${{AS_AGENT:-}}" ] || exit 89 ;;
  *) exit 90 ;;
esac
case " $* " in *" --background "*) ;; *) exit 91 ;; esac
case " $* " in *" --make-pidfile "*) ;; *) exit 92 ;; esac
case " $* " in *" --exec {layout.binary} "*) ;; *) exit 94 ;; esac
case " $* " in *" --startas /bin/sh "*) ;; *) exit 95 ;; esac
printf '%s\\n' "$@" > "{argv_path}"
while [ $# -gt 0 ]; do
  if [ "$1" = "--" ]; then
    shift
    touch "{started}"
    exec /bin/sh "$@"
  fi
  shift
done
exit 93
''',
        )
        (layout.etc / 'environment').write_text(
            '# comment\n'
            '; comment\n'
            '\n'
            'not-assignment\n'
            'DD_EXTRA=from-file\n'
            'DD_INSTALL_DIR=/wrong\n'
            'DD_CONF_DIR=/wrong\n'
            'AGENT_USER=root\n'
            'DD_QUOTED="hello world"\n'
            f'DD_PAYLOAD=$(touch {marker})\n'
            f'touch {marker}\n'
        )
        runtime = self.root / 'var-run' / 'datadog-procmgrd'
        rendered = self.root / 'init-script'
        script = render(source, layout.install, layout.etc)
        script = script.replace('/sbin/start-stop-daemon', str(commands / 'start-stop-daemon'))
        script = script.replace('"/etc/init.d/$NAME" load-env', f'"{rendered}" load-env')
        script = script.replace('. /lib/lsb/init-functions', f'. "{init_functions}"')
        script = script.replace('/etc/default/', f'{self.root}/default/')
        # The real path is /var/run, which this user cannot create. install still runs.
        script = script.replace(
            SYSV_RUNTIME_DIR,
            f'/usr/bin/install -d -o {os.getuid()} -g {os.getgid()} -m 0755 {runtime} || return 2',
        )
        rendered.write_text(script)

        env = self.base_env(f'{commands}:/usr/bin:/bin')
        result = subprocess.run(
            [SHELL, str(rendered), 'start'], env=env, text=True, capture_output=True, check=False, timeout=30
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(started.exists())
        self.assertTrue(runtime.is_dir())
        self.assertFalse(marker.exists())

        argv = argv_path.read_text().splitlines()
        self.assertEqual(argv[argv.index('--chuid') + 1], 'dd-agent')
        self.assertEqual(argv[argv.index('--exec') + 1], str(layout.binary))
        self.assertEqual(argv[argv.index('--startas') + 1], '/bin/sh')

        seen = parse_env(layout.env_out)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_QUOTED'), 'hello world')
        self.assertEqual(seen.get('DD_PAYLOAD'), f'$(touch {marker})')
        self.assertEqual(seen.get('AGENT_USER'), 'root')
        self.assertEqual(seen.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

    def test_debian_upstart_exports_environment_before_exec(self):
        text = UPSTART_DEBIAN.read_text()
        self.assertNotIn('\nsetuid ', f'\n{text}')
        self.assertNotIn('pre-start script', text)
        main = upstart_main_block(UPSTART_DEBIAN)
        self.assertLess(main.index(UPSTART_RUNTIME_DIR), main.index('--chuid dd-agent'))

        layout = Layout(self.root)
        commands = self.root / 'commands'
        runtime = self.root / 'var-run' / 'datadog-procmgrd'
        sysv = render(SYSV_PROCMGR.read_text(), layout.install, layout.etc)
        sysv_path = self.root / 'sysv-procmgr'
        sysv_path.write_text(sysv)
        sysv_path.chmod(0o755)
        # No --background, so the fake replaces itself with the load-env child.
        write_executable(
            commands / 'start-stop-daemon',
            '''case " $* " in *" --background "*) exit 96 ;; esac
case " $* " in *" --chuid dd-agent "*) ;; *) exit 97 ;; esac
while [ $# -gt 0 ]; do
  if [ "$1" = "--" ]; then
    shift
    exec /bin/sh "$@"
  fi
  shift
done
exit 98
''',
        )
        script = render(main, layout.install, layout.etc)
        script = script.replace(
            UPSTART_RUNTIME_DIR,
            f'/usr/bin/install -d -o {os.getuid()} -g {os.getgid()} -m 0755 {runtime} || exit 1',
        )
        script = script.replace('/etc/init.d/datadog-agent-procmgr', str(sysv_path))
        script_path = self.root / 'upstart-script.sh'
        script_path.write_text(script)

        result = subprocess.run(
            [SHELL, str(script_path)],
            env=self.base_env(f'{commands}:/usr/bin:/bin'),
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(runtime.is_dir())

        seen = parse_env(layout.env_out)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

    def test_redhat_upstart_drops_uid_before_loading_the_environment(self):
        template = UPSTART_REDHAT.read_text()
        self.assertNotIn('exec su', template)
        self.assertNotIn('runuser', template)
        self.assertNotIn('DD_PROCMGR_RUN_AS_USER', template)
        self.assertNotIn('. "{etc_dir}/environment"', template)
        self.assertNotIn('set -a', template)
        self.assertNotIn('exec "{install_dir}/embedded/bin/dd-procmgrd"', template)
        main = upstart_main_block(UPSTART_REDHAT)
        outer, delimiter, inner = main.partition("<<'END_LOAD_ENV'\n")
        self.assertEqual(delimiter, "<<'END_LOAD_ENV'\n")
        self.assertIn('exec /usr/sbin/chroot --userspec="dd-agent:$group" --groups="$groups"', outer)
        self.assertLess(outer.index('/usr/bin/id -G dd-agent'), outer.index("tr ' ' ','"))
        self.assertLess(outer.index("tr ' ' ','"), outer.index('exec /usr/sbin/chroot'))
        self.assertLess(outer.index('/usr/bin/id -gn dd-agent'), outer.index('exec /usr/sbin/chroot'))
        self.assertNotIn('export "$key=$value"', outer)
        self.assertNotIn('procmgr-errors.log', outer)
        self.assertIn('export "$key=$value"', inner)
        self.assertLess(inner.index('export "$key=$value"'), inner.index('export DD_LOG_TO_CONSOLE=false'))
        self.assertLess(inner.index('export DD_INSTALL_DIR="$install"'), inner.index('exec "$daemon"'))
        self.assertIn('exec "$daemon" >> /var/log/datadog/procmgr-errors.log 2>&1 </dev/null', inner)

        layout = Layout(self.root)
        marker = self.root / 'pwned'
        supervisor = self.root / 'supervisor-called'
        daemon_pid = self.root / 'daemon.pid'
        logfile = self.root / 'procmgr-errors.log'
        chroot_env = self.root / 'chroot.env'
        chroot_ids = self.root / 'chroot.ids'
        (layout.etc / 'environment').write_text(
            '# comment\n'
            '; comment\n'
            '\n'
            'not-assignment\n'
            'DD_EXTRA=from-file\n'
            'DD_INSTALL_DIR=/wrong\n'
            'DD_CONF_DIR=/wrong\n'
            'DD_LOG_TO_CONSOLE=true\n'
            'LD_PRELOAD=/tmp/not-a-library\n'
            'AGENT_USER=root\n'
            'DD_QUOTED="hello world"\n'
            f'DD_PAYLOAD=$(touch {marker})\n'
            f'touch {marker}\n'
        )
        write_executable(
            layout.binary,
            f'echo $$ > "{daemon_pid}"\necho daemon-stdout\nenv > "{layout.env_out}"\nexit 0\n',
        )
        script = render(main, layout.install, layout.etc)
        script = script.replace('/var/log/datadog/procmgr-errors.log', str(logfile))
        commands = self.root / 'commands'
        script = script.replace('/usr/sbin/chroot', str(commands / 'chroot'))
        script = script.replace('/usr/bin/id', str(commands / 'id'))
        script_path = self.root / 'upstart-script.sh'
        script_path.write_text(script)

        for name in ('su', 'runuser', 'sudo'):
            write_executable(commands / name, f'echo {name} > "{supervisor}"\nexit 99\n')
        write_executable(
            commands / 'id',
            'case "$*" in\n'
            '  "-G dd-agent") printf "%s\\n" "1000 10" ;;\n'
            '  "-gn dd-agent") printf "%s\\n" "dd-agent" ;;\n'
            '  *) exit 9 ;;\n'
            'esac\n',
        )
        write_executable(
            commands / 'chroot',
            f'''echo chroot-stdout
env > "{chroot_env}"
userspec=
groups=
while [ $# -gt 0 ]; do
  case $1 in
    --userspec=*) userspec=${{1#--userspec=}}; shift ;;
    --groups=*) groups=${{1#--groups=}}; shift ;;
    *) break ;;
  esac
done
[ -n "$userspec" ] || exit 4
[ -n "$groups" ] || exit 5
case $groups in *\\ *) exit 6 ;; esac
[ "$1" = / ] || exit 3
shift
printf '%s\\n' "$userspec" "$groups" > "{chroot_ids}"
exec "$@"
''',
        )

        proc = subprocess.Popen(
            [SHELL, str(script_path)],
            env=self.base_env(f'{commands}:/usr/bin:/bin'),
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        stdout, stderr = proc.communicate(timeout=30)
        log = logfile.read_text() if logfile.exists() else ''
        self.assertEqual(proc.returncode, 0, stdout + stderr + log)
        self.assertFalse(marker.exists())
        self.assertFalse(supervisor.exists())
        self.assertEqual(int(daemon_pid.read_text().strip()), proc.pid)
        self.assertIn('chroot-stdout', stdout)
        self.assertNotIn('daemon-stdout', stdout)
        self.assertIn('daemon-stdout', log)
        self.assertNotIn('chroot-stdout', log)
        self.assertEqual(chroot_ids.read_text().splitlines(), ['dd-agent:dd-agent', '1000,10'])

        launched = parse_env(chroot_env)
        self.assertNotIn('DD_EXTRA', launched)
        self.assertNotIn('LD_PRELOAD', launched)
        self.assertEqual(launched.get('DD_INSTALL_DIR'), 'from-parent')
        self.assertEqual(launched.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

        seen = parse_env(layout.env_out)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_QUOTED'), 'hello world')
        self.assertEqual(seen.get('DD_PAYLOAD'), f'$(touch {marker})')
        self.assertEqual(seen.get('DD_LOG_TO_CONSOLE'), 'false')
        self.assertEqual(seen.get('LD_PRELOAD'), '/tmp/not-a-library')
        self.assertEqual(seen.get('AGENT_USER'), 'root')
        self.assertNotIn('DD_PROCMGR_RUN_AS_USER', seen)
        self.assertEqual(seen.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

        write_executable(commands / 'id', 'exit 1\n')
        daemon_pid.unlink()
        layout.env_out.unlink(missing_ok=True)
        failed = subprocess.run(
            [SHELL, str(script_path)],
            env=self.base_env(f'{commands}:/usr/bin:/bin'),
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertNotEqual(failed.returncode, 0, failed.stdout + failed.stderr)
        self.assertFalse(daemon_pid.exists())
        self.assertFalse(layout.env_out.exists())

    def test_procmgr_checks_definitions_before_sourcing_the_admin_file(self):
        sysv = SYSV_PROCMGR.read_text()
        self.assertNotIn('/etc/datadog-agent/datadog-agent-procmgr', sysv)
        self.assertLess(sysv.index('procmgr_configured'), sysv.index('/etc/default/${NAME}'))
        self.assertIn('[ -f "$f" ]', sysv)
        debian = upstart_main_block(UPSTART_DEBIAN)
        self.assertNotIn('/etc/datadog-agent/datadog-agent-procmgr', debian)
        self.assertLess(debian.index('procmgr_configured'), debian.index('. /etc/default/datadog-agent-procmgr'))
        for header in ('script', 'pre-start script'):
            block = upstart_block(UPSTART_REDHAT, header)
            self.assertNotIn('/etc/datadog-agent/datadog-agent-procmgr', block)
            self.assertLess(block.index('procmgr_configured'), block.index('. /etc/default/datadog-agent-procmgr'))

    def test_upstart_follows_definitions_and_the_admin_file(self):
        admin = self.root / 'default' / 'datadog-agent-procmgr'
        admin.parent.mkdir()
        called = self.root / 'called'
        install = self.root / 'install'
        binary = install / 'embedded/bin/dd-procmgrd'
        binary.parent.mkdir(parents=True)
        binary.write_text('')
        binary.chmod(0o755)
        processes = install / 'processes.d'
        processes.mkdir()
        definition = processes / 'datadog-agent-par-control.yaml'
        directory_entry = processes / 'x.yaml'
        cases = (
            (UPSTART_DEBIAN, ('script',)),
            (UPSTART_REDHAT, ('pre-start script', 'script')),
        )
        # definition present, admin text, disabled
        files = (
            (False, None, True),
            (False, 'DATADOG_ENABLED=no\n', True),
            (False, 'DATADOG_ENABLED=yes\n', False),
            (True, 'DATADOG_ENABLED=no\n', True),
            (True, None, False),
            (True, 'DATADOG_ENABLED=yes\n', False),
        )
        for template, headers in cases:
            for header in headers:
                block = upstart_block(template, header)
                for present, admin_text, disabled in files:
                    with self.subTest(template=template.name, block=header, present=present, admin=admin_text):
                        if present:
                            definition.write_text('command: /bin/true\n')
                        else:
                            definition.unlink(missing_ok=True)
                        if admin_text is None:
                            admin.unlink(missing_ok=True)
                        else:
                            admin.write_text(admin_text)
                        called.unlink(missing_ok=True)
                        script = render(block, install, self.root / 'etc')
                        script = script.replace('/etc/default/datadog-agent-procmgr', str(admin))
                        reached = f'touch "{called}"; exit 0'
                        script = script.replace(UPSTART_RUNTIME_DIR, reached)
                        script = script.replace('groups=$(/usr/bin/id -G dd-agent) || exit 1', reached)
                        result = subprocess.run(
                            [SHELL, '-e', '-c', script],
                            text=True,
                            capture_output=True,
                            check=False,
                            timeout=15,
                        )
                        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                        self.assertEqual(called.exists(), not disabled)
                definition.unlink(missing_ok=True)
                directory_entry.mkdir()
                admin.unlink(missing_ok=True)
                called.unlink(missing_ok=True)
                script = render(block, install, self.root / 'etc')
                script = script.replace('/etc/default/datadog-agent-procmgr', str(admin))
                reached = f'touch "{called}"; exit 0'
                script = script.replace(UPSTART_RUNTIME_DIR, reached)
                script = script.replace('groups=$(/usr/bin/id -G dd-agent) || exit 1', reached)
                result = subprocess.run(
                    [SHELL, '-e', '-c', script], text=True, capture_output=True, check=False, timeout=15
                )
                directory_entry.rmdir()
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertFalse(called.exists())
                yml = processes / 'custom.yml'
                yml.write_text('command: /bin/true\n')
                called.unlink(missing_ok=True)
                result = subprocess.run(
                    [SHELL, '-e', '-c', script], text=True, capture_output=True, check=False, timeout=15
                )
                yml.unlink()
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertTrue(called.exists())

    def test_procmgr_stop_waits_for_par_control(self):
        sysv = SYSV_PROCMGR.read_text()
        self.assertIn('--retry=TERM/200/KILL/5', sysv)
        self.assertNotIn('--retry=30', sysv)
        for template in (UPSTART_DEBIAN, UPSTART_REDHAT):
            text = template.read_text()
            self.assertRegex(text, r'(?m)^kill timeout 200$')
            self.assertLess(text.index('kill timeout 200'), text.index('\nscript\n'))

    def test_runtime_dir_is_created_as_root_before_the_daemon(self):
        sysv = SYSV_PROCMGR.read_text()
        self.assertIn(SYSV_RUNTIME_DIR, sysv)
        self.assertLess(sysv.index(SYSV_RUNTIME_DIR), sysv.index('--chuid "$AGENT_USER"'))
        debian_main = upstart_main_block(UPSTART_DEBIAN)
        self.assertIn(UPSTART_RUNTIME_DIR, debian_main)
        self.assertLess(debian_main.index(UPSTART_RUNTIME_DIR), debian_main.index('--chuid dd-agent'))
        redhat_pre_start = upstart_block(UPSTART_REDHAT, 'pre-start script')
        self.assertIn(UPSTART_RUNTIME_DIR, redhat_pre_start)
        self.assertNotIn(UPSTART_RUNTIME_DIR, upstart_main_block(UPSTART_REDHAT))

    def sysv_harness(self, configured=True, setpriv=True, admin_defaults=None, extra_processes=()):
        """Render the procmgr SysV script with a fake start-stop-daemon that records its calls."""
        root = Path(tempfile.mkdtemp(dir=self.root))
        layout = Layout(root)
        if admin_defaults is not None:
            admin = root / 'default' / 'datadog-agent-procmgr'
            admin.parent.mkdir(parents=True, exist_ok=True)
            admin.write_text(admin_defaults)
        commands = root / 'commands'
        started = root / 'started'
        calls = root / 'calls'
        pidfile = layout.install / 'run' / 'dd-procmgrd.pid'
        init_functions = root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n'
            'log_daemon_msg() { :; }\n'
            'log_end_msg() { return "$1"; }\n'
            'log_success_msg() { echo "$*"; }\n'
            'log_failure_msg() { echo "$*"; }\n'
        )
        write_agent_commands(commands)
        write_executable(
            commands / 'start-stop-daemon',
            f'''echo "$1 as_agent=${{AS_AGENT:-no}}" >> "{calls}"
case "$1" in
  --status) [ "${{AS_AGENT:-}}" = yes ] || exit 91; [ -f "{started}" ] && exit 0 || exit 3 ;;
  --stop) [ "${{AS_AGENT:-}}" = yes ] || exit 91; exit "$STOP_RC" ;;
  --start) [ -z "${{AS_AGENT:-}}" ] || exit 89; touch "{started}"; exit 0 ;;
esac
exit 90
''',
        )
        if not configured:
            for path in (layout.install / 'processes.d').glob('*'):
                path.unlink()
        for name in extra_processes:
            (layout.install / 'processes.d' / name).write_text('command: /bin/true\n')
        runtime = root / 'var-run' / 'datadog-procmgrd'
        script = render(SYSV_PROCMGR.read_text(), layout.install, layout.etc)
        script = script.replace('. /lib/lsb/init-functions', f'. "{init_functions}"')
        script = script.replace('/etc/default/', f'{root}/default/')
        script = script.replace('/sbin/start-stop-daemon', str(commands / 'start-stop-daemon'))
        script = script.replace(
            SYSV_RUNTIME_DIR,
            f'/usr/bin/install -d -o {os.getuid()} -g {os.getgid()} -m 0755 {runtime} || return 2',
        )
        if not setpriv:
            script = script.replace('command -v setpriv > /dev/null 2>&1', 'false')
        rendered = root / 'init-script'
        rendered.write_text(script)
        env = self.base_env(f'{commands}:/usr/bin:/bin')

        def run(action, stop_rc=0, running=False, **extra):
            calls.unlink(missing_ok=True)
            started.unlink(missing_ok=True)
            if running:
                started.touch()
            result = subprocess.run(
                [SHELL, str(rendered), action],
                env={**env, 'STOP_RC': str(stop_rc), **extra},
                text=True,
                capture_output=True,
                check=False,
                timeout=30,
            )
            return result, calls.read_text().splitlines() if calls.exists() else []

        return run, started, pidfile

    def test_sysv_failed_stop_keeps_the_pidfile_and_blocks_restart(self):
        run, started, pidfile = self.sysv_harness()

        pidfile.write_text('4242\n')
        failed_stop, _ = run('stop', 2)
        self.assertNotEqual(failed_stop.returncode, 0, failed_stop.stdout + failed_stop.stderr)
        self.assertEqual(pidfile.read_text(), '4242\n')
        self.assertFalse(started.exists())

        failed_restart, _ = run('restart', 2)
        self.assertNotEqual(failed_restart.returncode, 0, failed_restart.stdout + failed_restart.stderr)
        self.assertEqual(pidfile.read_text(), '4242\n')
        self.assertFalse(started.exists())

        for stop_rc in (0, 1):
            with self.subTest(stop_rc=stop_rc):
                pidfile.write_text('4242\n')
                stopped, _ = run('stop', stop_rc)
                self.assertEqual(stopped.returncode, 0, stopped.stdout + stopped.stderr)
                self.assertFalse(pidfile.exists())
                self.assertFalse(started.exists())

    def test_sysv_status_checks_the_daemon_as_the_agent_user(self):
        run, _, _ = self.sysv_harness()
        result, calls = run('status', running=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('is running', result.stdout)
        self.assertEqual(calls, ['--status as_agent=yes'])

        run, _, _ = self.sysv_harness()
        result, calls = run('status')
        self.assertEqual(result.returncode, 3, result.stdout + result.stderr)
        self.assertIn('is not running', result.stdout)
        self.assertEqual(calls, ['--status as_agent=yes'])

    def test_sysv_checks_the_daemon_as_the_agent_user(self):
        for setpriv in (True, False):
            with self.subTest(setpriv=setpriv):
                run, started, _ = self.sysv_harness(setpriv=setpriv)
                result, calls = run('start')
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertTrue(started.exists())
                self.assertEqual(calls, ['--status as_agent=yes', '--start as_agent=no', '--status as_agent=yes'])
                result, calls = run('stop')
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(calls, ['--stop as_agent=yes'])

    def test_sysv_start_fails_when_the_agent_user_cannot_be_resolved(self):
        run, started, _ = self.sysv_harness()
        result, calls = run('start', ID_RC='1')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(started.exists())
        self.assertEqual(calls, [])

    def test_sysv_restart_stops_but_does_not_start_when_disabled(self):
        run, started, pidfile = self.sysv_harness(configured=False)
        pidfile.write_text('4242\n')
        result, calls = run('restart')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('Disabled via process manager configuration', result.stdout)
        self.assertEqual(calls, ['--stop as_agent=yes'])
        self.assertFalse(pidfile.exists())
        self.assertFalse(started.exists())

    def test_sysv_start_follows_definitions_and_the_admin_file(self):
        cases = (
            ('definitions present', True, (), None, True),
            ('empty catalog', False, (), None, False),
            ('stray definition only', False, ('custom.yml',), None, True),
            ('admin disables', True, (), 'DATADOG_ENABLED=no\n', False),
            ('admin enables', False, (), 'DATADOG_ENABLED=yes\n', True),
        )
        for name, configured, extra, admin, should_start in cases:
            with self.subTest(name):
                run, started, _ = self.sysv_harness(configured=configured, admin_defaults=admin, extra_processes=extra)
                result, calls = run('start')
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(started.exists(), should_start)
                self.assertEqual('--start as_agent=no' in calls, should_start)
                self.assertEqual('Disabled via process manager configuration' in result.stdout, not should_start)

    def test_sysv_start_fails_when_runtime_dir_cannot_be_created(self):
        layout = Layout(self.root)
        commands = self.root / 'commands'
        started = self.root / 'started'
        init_functions = self.root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n' 'log_daemon_msg() { :; }\n' 'log_end_msg() { return "$1"; }\n'
        )
        write_executable(commands / 'install', 'exit 1\n')
        write_executable(commands / 'start-stop-daemon', f'touch "{started}"\nexit 0\n')
        script = render(SYSV_PROCMGR.read_text(), layout.install, layout.etc)
        script = script.replace('. /lib/lsb/init-functions', f'. "{init_functions}"')
        script = script.replace('/etc/default/', f'{self.root}/default/')
        script = script.replace('/usr/bin/install', str(commands / 'install'))
        rendered = self.root / 'init-script'
        rendered.write_text(script)
        env = self.base_env(f'{commands}:/usr/bin:/bin')
        result = subprocess.run(
            [SHELL, str(rendered), 'start'], env=env, text=True, capture_output=True, check=False, timeout=30
        )
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(started.exists())

    def test_templates_do_not_force_split_mode(self):
        for template in (SYSV_PROCMGR, UPSTART_DEBIAN, UPSTART_REDHAT):
            with self.subTest(template=template.name):
                self.assertNotIn('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED', template.read_text())

    def test_sysv_core_agent_starts_and_stops_procmgr(self):
        text = SYSV_AGENT.read_text()
        should_start = re.search(r'^# Should-Start:(.*)$', text, re.MULTILINE)
        should_stop = re.search(r'^# Should-Stop:(.*)$', text, re.MULTILINE)
        self.assertIsNotNone(should_start)
        self.assertIsNotNone(should_stop)
        self.assertIn('datadog-agent-procmgr', should_start.group(1).split())
        self.assertIn('datadog-agent-procmgr', should_stop.group(1).split())
        self.assertEqual(text.count('service datadog-agent-procmgr start'), 1)
        self.assertEqual(text.count('service datadog-agent-procmgr stop'), 1)
        self.assertLess(
            text.index('service datadog-agent-procmgr start'), text.index('service datadog-agent-action start')
        )
        self.assertLess(
            text.index('service datadog-agent-action stop'), text.index('service datadog-agent-procmgr stop')
        )
        # Boot starts each init script on its own, so the action script declares the same order as the core script.
        action = SYSV_ACTION.read_text()
        action_start = re.search(r'^# Should-Start:(.*)$', action, re.MULTILINE)
        action_stop = re.search(r'^# Should-Stop:(.*)$', action, re.MULTILINE)
        self.assertIsNotNone(action_start)
        self.assertIsNotNone(action_stop)
        self.assertIn('datadog-agent-procmgr', action_start.group(1).split())
        self.assertIn('datadog-agent-procmgr', action_stop.group(1).split())

    def test_packaging_wires_expected_files(self):
        debian = (REPO_ROOT / 'packages/debian/etc/BUILD.bazel').read_text()
        redhat = (REPO_ROOT / 'packages/redhat/etc/BUILD.bazel').read_text()
        omnibus = (REPO_ROOT / 'omnibus/config/software/init-scripts-agent.rb').read_text()
        self.assertIn('sysvinit_debian.procmgr.in', debian)
        self.assertIn('upstart_debian.procmgr.conf.in', debian)
        self.assertIn('upstart_redhat.procmgr.conf.in', redhat)
        self.assertIn("/etc/init.d/datadog-agent-procmgr'", omnibus)
        self.assertIn("/etc/init/datadog-agent-procmgr.conf'", omnibus)
        self.assertFalse((REPO_ROOT / 'packages/redhat/etc/sysvinit_redhat.procmgr.in').exists())
        self.assertNotIn('sysvinit_redhat', redhat)

    def test_shell_scripts_parse(self):
        shell = shutil.which('sh')
        self.assertIsNotNone(shell)
        sysv = render(SYSV_PROCMGR.read_text(), self.root / 'i', self.root / 'e')
        path = self.root / 'sysv.sh'
        path.write_text(sysv)
        subprocess.run([shell, '-n', str(path)], check=True)
        path.write_text(render(upstart_main_block(UPSTART_DEBIAN), self.root / 'i', self.root / 'e'))
        subprocess.run([shell, '-n', str(path)], check=True)
        for header in ('script', 'pre-start script'):
            path.write_text(render(upstart_block(UPSTART_REDHAT, header), self.root / 'i', self.root / 'e'))
            subprocess.run([shell, '-n', str(path)], check=True)


if __name__ == '__main__':
    unittest.main()
