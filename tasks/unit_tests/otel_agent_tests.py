import unittest
from unittest.mock import call, mock_open, patch

from invoke import Context

import tasks.otel_agent as otel_agent


class TestOTelAgentTasks(unittest.TestCase):
    dockerfile = None

    def setUp(self):
        with open(otel_agent.DDOT_BYOC_DOCKERFILE) as f:
            self.dockerfile = f.read()

    def tearDown(self):
        pass

    def assert_mock_with_calls(self, mock, calls):
        all_calls = mock.mock_calls
        for c in calls:
            if c not in all_calls:
                return False

        return True

    def test_byoc_release(self):
        image = "foo"
        branch = "x.y.z"
        repo = "bar"

        c = Context()
        m = mock_open(read_data=self.dockerfile)
        with patch("builtins.open", m):
            otel_agent.byoc_release(c, image=image, branch=branch, repo=repo)

        expected_calls = [
            call(otel_agent.DDOT_BYOC_DOCKERFILE, "w"),
            call().write(f"ARG AGENT_REPO={repo}\n"),
            call().write(f"ARG AGENT_VERSION={image}\n"),
            call().write(f"ARG AGENT_BRANCH={branch}\n"),
        ]

        self.assertEqual(self.assert_mock_with_calls(m, expected_calls), True)
