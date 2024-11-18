import unittest

from invoke import Context

from tasks.libs.common.agent6 import agent_context, is_agent6


def get_ctx():
    return Context()


class TestAgent6(unittest.TestCase):
    def test_context_is_agent6_true(self):
        with agent_context(get_ctx(), 6):
            self.assertTrue(is_agent6())

    def test_context_is_agent6_false(self):
        with agent_context(get_ctx(), 7):
            self.assertFalse(is_agent6())
