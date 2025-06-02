import os
import re
import shutil
import unittest
from pathlib import Path
from unittest.mock import patch

from invoke.context import Context, MockContext, Result

from tasks.libs.common.gomodules import Configuration, get_default_modules
from tasks.libs.common.worktree import agent_context, init_env, is_worktree

TEST_WORKTREE_DIR = Path('/tmp/datadog-agent-worktree')


def get_ctx():
    return Context()


class WorktreeMockContext(MockContext):
    """Mock for invoke context to simulate git commands (clone / switch).

    Only `modules.yml` will be updated to simulate it.
    """

    def __init__(self):
        super().__init__()

        self.branch = None
        self.modules_main = get_default_modules()
        # Exclude some modules
        self.modules_653x = {name: mod for (name, mod) in get_default_modules().items() if not name.startswith('comp/')}

        assert self.modules_main.keys() != self.modules_653x.keys()

    def _checkout(self, branch):
        assert branch in ('main', '6.53.x')

        if self.branch == branch:
            return

        self.branch = branch
        modules = self.modules_main if branch == 'main' else self.modules_653x

        Configuration(TEST_WORKTREE_DIR, modules, set()).to_file()

    def _clone(self):
        self.reset()
        TEST_WORKTREE_DIR.mkdir(parents=True)
        self._checkout('main')

    def reset(self):
        shutil.rmtree(TEST_WORKTREE_DIR, ignore_errors=True)
        self.branch = None

    def run(self, command, *args, **kwargs):
        if re.match(r'git.*rev-parse.*', command):
            return Result(stdout=self.branch)

        if re.match(r'git.*remote get-url.*', command):
            return Result(stdout='git@my-amazing-git-server.dev:datadog/datadog-agent.git')

        if (
            re.match(r'git.*status --porcelain.*', command)
            or re.match(r'git.*reset --hard.*', command)
            or re.match(r'git.*clean -f.*', command)
            or re.match(r'git.*branch.*', command)
            or re.match(r'cp.*', command)
        ):
            return Result(stdout='')

        if re.match(r'git.*clone.*', command):
            self._clone()

            return Result(stdout='cloned')

        if re.match(r'git.*checkout.*', command):
            on_653x = '6.53.x' in command

            self._checkout('6.53.x' if on_653x else 'main')

            return Result(stdout='checked out')

        return super().run(command, *args, **kwargs)


class TestWorktree(unittest.TestCase):
    def setUp(self):
        # Pull only once
        self.mock_ctx = WorktreeMockContext()
        self.mock_ctx.reset()
        self.patch_workdir = patch('tasks.libs.common.worktree.WORKTREE_DIRECTORY', TEST_WORKTREE_DIR)
        self.patch_workdir.start()
        self.patch_agentdir = patch('tasks.libs.common.gomodules.agent_working_directory', lambda: TEST_WORKTREE_DIR)
        self.patch_agentdir.start()

        os.environ['AGENT_WORKTREE_NO_PULL'] = '1'
        init_env(self.mock_ctx, '6.53.x')

    def tearDown(self):
        self.patch_workdir.stop()
        self.patch_agentdir.stop()
        self.mock_ctx.reset()

    def test_context_is_worktree_true(self):
        with agent_context(self.mock_ctx, '6.53.x'):
            self.assertTrue(is_worktree())

    def test_context_is_worktree_false(self):
        self.assertFalse(is_worktree())

    def test_context_nested(self):
        with agent_context(self.mock_ctx, '6.53.x'):
            with agent_context(self.mock_ctx, '6.53.x'):
                self.assertTrue(is_worktree())
            self.assertTrue(is_worktree())

    def test_context_pwd(self):
        ctx = get_ctx()

        with agent_context(self.mock_ctx, None, skip_checkout=True):
            pwdnone = ctx.run('pwd').stdout

        with agent_context(self.mock_ctx, '6.53.x'):
            pwd6 = ctx.run('pwd').stdout

        with agent_context(self.mock_ctx, 'main'):
            pwdmain = ctx.run('pwd').stdout

        self.assertEqual(pwd6, pwdnone)
        self.assertEqual(pwd6, pwdmain)

    def test_context_modules(self):
        with agent_context(self.mock_ctx, 'main'):
            modules7 = get_default_modules()

        with agent_context(self.mock_ctx, '6.53.x'):
            modules6 = get_default_modules()

        self.assertNotEqual(set(modules6.keys()), set(modules7.keys()))

    def test_context_no_checkout(self):
        with agent_context(self.mock_ctx, 'main'):
            modules7 = get_default_modules()

        with agent_context(self.mock_ctx, '6.53.x'):
            modules6 = get_default_modules()

        # Cannot skip checkout if the branch is not the target one (current is 6.53.x)
        self.assertRaises(AssertionError, lambda: agent_context(self.mock_ctx, 'main', skip_checkout=True).__enter__())

        with agent_context(self.mock_ctx, '6.53.x', skip_checkout=True):
            modules_no_checkout = get_default_modules()

        self.assertNotEqual(set(modules6.keys()), set(modules7.keys()))
        self.assertEqual(set(modules6.keys()), set(modules_no_checkout.keys()))
