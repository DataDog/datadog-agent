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
SYSV_AGENT = REPO_ROOT / 'packages/debian/etc/sysvinit_debian.in'
UPSTART_DEBIAN = REPO_ROOT / 'packages/debian/etc/upstart_debian.procmgr.conf.in'
UPSTART_REDHAT = REPO_ROOT / 'packages/redhat/etc/upstart_redhat.procmgr.conf.in'
FOUR_VARIABLES = ('DD_PM_CONFIG_DIR', 'DD_CONF_DIR', 'DD_PID_DIR', 'DD_INSTALL_DIR')
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
        layout = Layout(self.root)
        commands = self.root / 'commands'
        started = self.root / 'started'
        start_env = self.root / 'start.env'
        init_functions = self.root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n'
            'log_daemon_msg() { :; }\n'
            'log_end_msg() { return "$1"; }\n'
            'log_success_msg() { :; }\n'
            'log_failure_msg() { :; }\n'
        )
        # Records the environment of the real start. The test call returns 0 before the daemon is
        # started and 1 after, which is how the script detects a running daemon.
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
env > "{start_env}"
touch "{started}"
exit 0
''',
        )
        runtime = self.root / 'var-run' / 'datadog-procmgrd'
        script = render(SYSV_PROCMGR.read_text(), layout.install, layout.etc)
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

        seen = parse_env(start_env)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

    def test_debian_upstart_exports_environment_before_exec(self):
        layout = Layout(self.root)
        script = render(upstart_main_block(UPSTART_DEBIAN), layout.install, layout.etc)
        script_path = self.root / 'upstart-script.sh'
        script_path.write_text(script)

        result = subprocess.run(
            ['/bin/sh', str(script_path)],
            env=self.base_env(os.environ['PATH']),
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        seen = parse_env(layout.env_out)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED'), PARENT_SPLIT)

    def test_redhat_upstart_sets_environment_inside_su_command(self):
        layout = Layout(self.root)
        logfile = self.root / 'procmgr-errors.log'
        script = render(upstart_main_block(UPSTART_REDHAT), layout.install, layout.etc)
        script = script.replace('/var/log/datadog/procmgr-errors.log', str(logfile))
        script_path = self.root / 'upstart-script.sh'
        script_path.write_text(script)

        commands = self.root / 'commands'
        # su without -m drops the environment, so the fake clears it before running the shell.
        write_executable(
            commands / 'su',
            '''[ "$1" = -s ] && [ "$3" = -c ] && [ "$5" = dd-agent ] && [ "$6" = -- ] || exit 97
shell=$2
command=$4
shift 6
exec env -i PATH="$PATH" "$shell" -c "$command" "$@"
''',
        )

        result = subprocess.run(
            ['/bin/sh', str(script_path)],
            env=self.base_env(f'{commands}:/usr/bin:/bin'),
            text=True,
            capture_output=True,
            check=False,
            timeout=30,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        seen = parse_env(layout.env_out)
        for key, value in layout.expected_four().items():
            self.assertEqual(seen.get(key), value, key)
        self.assertEqual(seen.get('DD_EXTRA'), 'from-file')
        self.assertEqual(seen.get('DD_LOG_TO_CONSOLE'), 'false')
        self.assertNotIn('DD_PRIVATE_ACTION_RUNNER_SPLIT_ENABLED', seen)

    def test_redhat_su_command_contains_all_variables(self):
        template = UPSTART_REDHAT.read_text()
        match = re.search(r"exec su -s /bin/sh -c '(.*?)' dd-agent --", template, re.DOTALL)
        self.assertIsNotNone(match, 'su -c command not found in the Red Hat procmgr job')
        command = match.group(1)
        for name in (*FOUR_VARIABLES, 'DD_LOG_TO_CONSOLE'):
            self.assertIn(f'{name}=', command)
        self.assertIn('. "{etc_dir}/environment"', command)

    def test_runtime_dir_is_created_as_root_before_the_daemon(self):
        sysv = SYSV_PROCMGR.read_text()
        self.assertIn(SYSV_RUNTIME_DIR, sysv)
        self.assertLess(sysv.index(SYSV_RUNTIME_DIR), sysv.index('--chuid'))
        for template in (UPSTART_DEBIAN, UPSTART_REDHAT):
            with self.subTest(template=template.name):
                pre_start = upstart_block(template, 'pre-start script')
                self.assertIn(UPSTART_RUNTIME_DIR, pre_start)
                self.assertNotIn(UPSTART_RUNTIME_DIR, upstart_main_block(template))

    def test_sysv_start_fails_when_runtime_dir_cannot_be_created(self):
        layout = Layout(self.root)
        commands = self.root / 'commands'
        started = self.root / 'started'
        init_functions = self.root / 'init-functions'
        init_functions.write_text(
            'init_is_upstart() { return 1; }\n'
            'log_daemon_msg() { :; }\n'
            'log_end_msg() { return "$1"; }\n'
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
        self.assertLess(text.index('service datadog-agent-procmgr start'), text.index('service datadog-agent-action start'))
        self.assertLess(text.index('service datadog-agent-action stop'), text.index('service datadog-agent-procmgr stop'))

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
        for template in (UPSTART_DEBIAN, UPSTART_REDHAT):
            for header in ('script', 'pre-start script'):
                path.write_text(render(upstart_block(template, header), self.root / 'i', self.root / 'e'))
                subprocess.run([shell, '-n', str(path)], check=True)


if __name__ == '__main__':
    unittest.main()
