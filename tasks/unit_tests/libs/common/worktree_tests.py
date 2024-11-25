import os
import unittest

from invoke import Context

from tasks.libs.common.git import get_default_branch
from tasks.libs.common.gomodules import get_default_modules
from tasks.libs.common.worktree import agent_context, init_env, is_worktree


def get_ctx():
    return Context()


class TestWorktree(unittest.TestCase):
    def setUp(self):
        print('SETUP')
        # Pull only once
        init_env(get_ctx(), '6.53.x')
        os.environ['AGENT_WORKTREE_NO_PULL'] = '1'

    def test_context_is_worktree_true(self):
        with agent_context(get_ctx(), '6.53.x'):
            self.assertTrue(is_worktree())

    def test_context_is_worktree_false(self):
        with agent_context(get_ctx(), None):
            self.assertFalse(is_worktree())

    def test_context_nested(self):
        with agent_context(get_ctx(), '6.53.x'):
            with agent_context(get_ctx(), '6.53.x'):
                self.assertTrue(is_worktree())
            self.assertTrue(is_worktree())

    def test_context_pwd(self):
        ctx = get_ctx()

        with agent_context(ctx, None):
            pwdlocal = ctx.run('pwd').stdout

        with agent_context(ctx, '6.53.x'):
            pwd6 = ctx.run('pwd').stdout

        self.assertNotEqual(pwd6, pwdlocal)

    def test_context_modules(self):
        ctx = get_ctx()

        with agent_context(ctx, 'main'):
            modules7 = get_default_modules()

        with agent_context(ctx, '6.53.x'):
            modules6 = get_default_modules()

        self.assertNotEqual(set(modules6.keys()), set(modules7.keys()))

    def test_context_branch(self):
        ctx = get_ctx()

        with agent_context(ctx, 'main'):
            branch7 = get_default_branch()

        with agent_context(ctx, '6.53.x'):
            branch6 = get_default_branch()

        self.assertNotEqual(branch6, branch7)

    def test_context_no_checkout(self):
        ctx = get_ctx()

        with agent_context(ctx, '6.53.x'):
            branch6 = get_default_branch()

        with agent_context(ctx, 'main'):
            branch7 = get_default_branch()

        with agent_context(ctx, 'main', skip_checkout=True):
            branch_no_checkout = get_default_branch()

        self.assertNotEqual(branch6, branch7)
        self.assertEqual(branch7, branch_no_checkout)
