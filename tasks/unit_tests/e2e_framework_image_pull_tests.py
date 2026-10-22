import unittest
from unittest import mock

from invoke.runners import Result

from tasks.e2e_framework.aws.deploy import _image_pull_flags
from tasks.e2e_framework.image_pull import AGENT_QA_ECR, SANDBOX_ECR


def _ctx(*results: Result):
    """A context whose `run` returns the given results in order."""
    ctx = mock.MagicMock()
    ctx.run.side_effect = list(results)
    return ctx


class TestImagePullFlags(unittest.TestCase):
    def test_default_image_gets_agent_qa_credentials(self):
        # The default Agent image and the workloads come from the agent-qa cache.
        flags = _image_pull_flags(_ctx(Result(stdout="qa-token\n")), None, None)
        self.assertEqual(
            flags,
            {
                "ddagent:imagePullRegistry": AGENT_QA_ECR,
                "ddagent:imagePullUsername": "AWS",
                "ddagent:imagePullPassword": "qa-token",
            },
        )

    def test_sandbox_image_adds_sandbox_credentials(self):
        ctx = _ctx(Result(stdout="qa-token"), Result(stdout="sandbox-token"))
        flags = _image_pull_flags(ctx, f"{SANDBOX_ECR}/agent-e2e-tests:me", None)
        self.assertEqual(flags["ddagent:imagePullRegistry"], f"{AGENT_QA_ECR},{SANDBOX_ECR}")
        self.assertEqual(flags["ddagent:imagePullUsername"], "AWS,AWS")
        self.assertEqual(flags["ddagent:imagePullPassword"], "qa-token,sandbox-token")

    def test_sandbox_cluster_agent_image_adds_sandbox_credentials(self):
        ctx = _ctx(Result(stdout="qa-token"), Result(stdout="sandbox-token"))
        flags = _image_pull_flags(ctx, None, f"{SANDBOX_ECR}/cluster-agent:me")
        self.assertEqual(flags["ddagent:imagePullRegistry"], f"{AGENT_QA_ECR},{SANDBOX_ECR}")

    def test_missing_agent_qa_access_still_returns_sandbox_credentials(self):
        ctx = _ctx(Result(stdout="", exited=1), Result(stdout="sandbox-token"))
        with mock.patch("builtins.print"):
            flags = _image_pull_flags(ctx, f"{SANDBOX_ECR}/agent-e2e-tests:me", None)
        self.assertEqual(flags["ddagent:imagePullRegistry"], SANDBOX_ECR)
        self.assertEqual(flags["ddagent:imagePullPassword"], "sandbox-token")

    def test_no_credentials_returns_no_flags(self):
        with mock.patch("builtins.print"):
            flags = _image_pull_flags(_ctx(Result(stdout="", exited=1)), None, None)
        self.assertEqual(flags, {})
