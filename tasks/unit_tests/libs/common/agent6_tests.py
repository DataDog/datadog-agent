import os
import unittest

from invoke import Context

from tasks.libs.common.agent6 import agent_context, init_env, is_agent6
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.gomodules import get_default_modules


def get_ctx():
    return Context()


class TestAgent6(unittest.TestCase):
    def setUp(self):
        # Pull only once
        init_env(get_ctx())
        os.environ['AGENT6_NO_PULL'] = '1'

    def test_context_is_agent6_true(self):
        with agent_context(get_ctx(), 6):
            self.assertTrue(is_agent6())

    def test_context_is_agent6_false(self):
        with agent_context(get_ctx(), 7):
            self.assertFalse(is_agent6())

    def test_context_nested(self):
        with agent_context(get_ctx(), 6):
            with agent_context(get_ctx(), 6):
                self.assertTrue(is_agent6())
            self.assertTrue(is_agent6())

    def test_context_pwd(self):
        ctx = get_ctx()

        with agent_context(ctx, 7):
            pwd7 = ctx.run('pwd').stdout

        with agent_context(ctx, 6):
            pwd6 = ctx.run('pwd').stdout

        self.assertNotEqual(pwd6, pwd7)

    def test_context_modules(self):
        ctx = get_ctx()

        with agent_context(ctx, 7):
            modules7 = get_default_modules()

        with agent_context(ctx, 6):
            modules6 = get_default_modules()

        self.assertNotEqual(set(modules6.keys()), set(modules7.keys()))

    def test_context_branch(self):
        ctx = get_ctx()

        with agent_context(ctx, 7):
            branch7 = get_default_branch()

        with agent_context(ctx, 6):
            branch6 = get_default_branch()

        self.assertNotEqual(branch6, branch7)
