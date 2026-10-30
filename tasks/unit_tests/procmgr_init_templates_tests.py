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
        self.assertIn('"$0" load-env', source)

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
        # --test reports the daemon running once the pid marker exists. The real start
        # executes the child shell, which is where the environment file is applied.
        write_executable(
            commands / 'start-stop-daemon',
            f'''case " $* " in
  *" --test "*) [ -f "{started}" ] && exit 1 || exit 0 ;;
esac
case " $* " in
  *" --start "*) ;;
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
        script = render(source, layout.install, layout.etc)
        script = script.replace('. /lib/lsb/init-functions', f'. "{init_functions}"')
        script = script.replace('/etc/default/', f'{self.root}/default/')
        # The real path is /var/run, which this user cannot create. install still runs.
        script = script.replace(
            SYSV_RUNTIME_DIR,
            f'/usr/bin/install -d -o {os.getuid()} -g {os.getgid()} -m 0755 {runtime} || return 2',
        )
        rendered = self.root / 'init-script'
        rendered.write_text(script)

        env = self.base_env(f'{commands}:/usr/bin:/bin')
        result = subprocess.run(
            ['/bin/sh', str(rendered), 'start'], env=env, text=True, capture_output=True, check=False, timeout=30
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
            ['/bin/sh', str(script_path)],
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
            ['/bin/sh', str(script_path)],
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
            ['/bin/sh', str(script_path)],
            env=self.base_env(f'{commands}:/usr/bin:/bin'),
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertNotEqual(failed.returncode, 0, failed.stdout + failed.stderr)
        self.assertFalse(daemon_pid.exists())
        self.assertFalse(layout.env_out.exists())

    def test_upstart_exits_when_the_defaults_file_disables_procmgr(self):
        defaults = self.root / 'default' / 'datadog-agent-procmgr'
        defaults.parent.mkdir()
        called = self.root / 'called'
        install = self.root / 'install'
        binary = install / 'embedded/bin/dd-procmgrd'
        binary.parent.mkdir(parents=True)
        binary.write_text('')
        binary.chmod(0o755)
        cases = (
            (UPSTART_DEBIAN, ('script',)),
            (UPSTART_REDHAT, ('pre-start script', 'script')),
        )
        for template, headers in cases:
            for header in headers:
                block = upstart_block(template, header)
                for contents, disabled in (('DATADOG_ENABLED=no\n', True), ('DATADOG_ENABLED=yes\n', False)):
                    with self.subTest(template=template.name, block=header, disabled=disabled):
                        defaults.write_text(contents)
                        called.unlink(missing_ok=True)
                        script = render(block, install, self.root / 'etc')
                        script = script.replace('/etc/default/datadog-agent-procmgr', str(defaults))
                        reached = f'touch "{called}"; exit 0'
                        script = script.replace(UPSTART_RUNTIME_DIR, reached)
                        script = script.replace('groups=$(/usr/bin/id -G dd-agent) || exit 1', reached)
                        result = subprocess.run(
                            ['/bin/sh', '-c', script],
                            text=True,
                            capture_output=True,
                            check=False,
                            timeout=15,
                        )
                        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                        self.assertEqual(called.exists(), not disabled)

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

    def test_sysv_failed_stop_keeps_the_pidfile_and_blocks_restart(self):
        layout = Layout(self.root)
        commands = self.root / 'commands'
        started = self.root / 'started'
        pidfile = layout.install / 'run' / 'dd-procmgrd.pid'
        init_functions = self.root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n' 'log_daemon_msg() { :; }\n' 'log_end_msg() { return "$1"; }\n'
        )
        write_executable(
            commands / 'start-stop-daemon',
            f'''case " $* " in
  *" --stop "*) exit "$STOP_RC" ;;
esac
touch "{started}"
exit 0
''',
        )
        script = render(SYSV_PROCMGR.read_text(), layout.install, layout.etc)
        script = script.replace('. /lib/lsb/init-functions', f'. "{init_functions}"')
        script = script.replace('/etc/default/', f'{self.root}/default/')
        rendered = self.root / 'init-script'
        rendered.write_text(script)
        env = self.base_env(f'{commands}:/usr/bin:/bin')

        def run(action, stop_rc):
            started.unlink(missing_ok=True)
            pidfile.write_text('4242\n')
            result = subprocess.run(
                ['/bin/sh', str(rendered), action],
                env={**env, 'STOP_RC': str(stop_rc)},
                text=True,
                capture_output=True,
                check=False,
                timeout=30,
            )
            return result

        failed_stop = run('stop', 2)
        self.assertNotEqual(failed_stop.returncode, 0, failed_stop.stdout + failed_stop.stderr)
        self.assertEqual(pidfile.read_text(), '4242\n')
        self.assertFalse(started.exists())

        failed_restart = run('restart', 2)
        self.assertNotEqual(failed_restart.returncode, 0, failed_restart.stdout + failed_restart.stderr)
        self.assertEqual(pidfile.read_text(), '4242\n')
        self.assertFalse(started.exists())

        for stop_rc in (0, 1):
            with self.subTest(stop_rc=stop_rc):
                stopped = run('stop', stop_rc)
                self.assertEqual(stopped.returncode, 0, stopped.stdout + stopped.stderr)
                self.assertFalse(pidfile.exists())
                self.assertFalse(started.exists())

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
            ['/bin/sh', str(rendered), 'start'], env=env, text=True, capture_output=True, check=False, timeout=30
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
        # Boot starts each init script on its own. The action script must follow procmgr or its one check selects monolithic mode.
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
