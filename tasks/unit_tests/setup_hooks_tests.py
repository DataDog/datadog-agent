import os
import shutil
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

from invoke import Context, Result

from tasks import setup
from tasks.libs.common.status import Status
from tasks.libs.common.utils import join_command
from tasks.unit_tests.pre_commit_foundation_tests import GitFixture

REPOSITORY = Path(__file__).resolve().parents[2]


class TestSetupHooks(unittest.TestCase):
    def context(self, fail=False):
        ctx = MagicMock(spec=Context)

        def run(command, **kwargs):
            if command == 'git config --global core.hooksPath':
                return Result(stdout='C:/hook path\n')
            if fail and command.startswith('pre-commit install '):
                raise RuntimeError('installation failed')
            return Result()

        ctx.run.side_effect = run
        return ctx

    @patch('tasks.setup.running_in_pyapp', return_value=False)
    def test_install_migrates_pre_push_and_restores_global_path(self, _):
        ctx = self.context()
        self.assertEqual(setup.pre_commit.body(ctx, interactive=False), (Status.OK, ''))
        commands = [call.args[0] for call in ctx.run.call_args_list]
        self.assertIn('pre-commit uninstall --hook-type pre-push', commands)
        self.assertIn('pre-commit uninstall --hook-type pre-commit', commands)
        self.assertEqual(commands[-1], join_command(['git', 'config', '--global', 'core.hooksPath', 'C:/hook path']))
        install = next(call for call in ctx.run.call_args_list if call.args[0].startswith('pre-commit install '))
        self.assertIn('--hook-type pre-commit', install.args[0])
        self.assertEqual(install.kwargs['env'], {'GOFLAGS': '-buildvcs=false'})

    @patch('tasks.setup.running_in_pyapp', return_value=False)
    def test_install_failure_restores_global_path(self, _):
        ctx = self.context(fail=True)
        with self.assertRaisesRegex(RuntimeError, 'installation failed'):
            setup.pre_commit.body(ctx, interactive=False)
        self.assertEqual(
            ctx.run.call_args.args[0], join_command(['git', 'config', '--global', 'core.hooksPath', 'C:/hook path'])
        )


class TestSetupNativeHooks(GitFixture):
    @unittest.skipUnless(shutil.which('pre-commit'), 'requires pre-commit')
    @patch('tasks.setup.running_in_pyapp', return_value=False)
    def test_real_setup_removes_previous_pre_push(self, _):
        self.stage('.pre-commit-config.yaml', (REPOSITORY / '.pre-commit-config.yaml').read_bytes())
        self.git('config', '--unset', 'core.hooksPath')
        with patch.dict(
            os.environ,
            {
                'GIT_CONFIG_GLOBAL': str(Path('.git/test-global').resolve()),
                'PRE_COMMIT_HOME': str(Path('.git/pre-commit-cache').resolve()),
            },
        ):
            self.ctx.run('pre-commit install --hook-type pre-push', hide=True)
            self.git('config', '--global', 'core.hooksPath', 'C:/a path/hooks')
            setup.pre_commit.body(self.ctx, interactive=False)
            self.assertFalse(Path('.git/hooks/pre-push').exists())
            self.assertTrue(Path('.git/hooks/pre-commit').is_file())
            self.assertEqual(self.git('config', '--global', 'core.hooksPath').strip(), b'C:/a path/hooks')
