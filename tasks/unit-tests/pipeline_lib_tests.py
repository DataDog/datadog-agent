import unittest
from unittest.mock import MagicMock

from gitlab.v4.objects import ProjectJob

from tasks.libs.pipeline import notifications
from tasks.libs.pipeline.data import get_job_failure_context
from tasks.libs.types.types import FailedJobReason, FailedJobType


class TestLoadAndValidate(unittest.TestCase):
    def test_files_loaded_correctly(self):
        # Assert that a couple of expected entries are there, including one that uses DEFAULT_JIRA_PROJECT
        self.assertEqual(notifications.GITHUB_JIRA_MAP['@datadog/agent-all'], "AGNTR")
        self.assertEqual(notifications.GITHUB_JIRA_MAP['@datadog/agent-ci-experience'], "ACIX")

        # Assert that a couple of expected entries are there, including one that uses DEFAULT_SLACK_PROJECT
        self.assertEqual(notifications.GITHUB_SLACK_MAP['@datadog/agent-all'], "#datadog-agent-pipelines")
        self.assertEqual(notifications.GITHUB_SLACK_MAP['@datadog/agent-ci-experience'], "#agent-devx-ops")


class TestFailedJobs(unittest.TestCase):
    def test_infra_failure(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "runner_system_failure",
            },
        )
        log = 'Empty log'

        fail_type, _fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.INFRA_FAILURE)

    def test_infra_failure_log(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "script_failure",
            },
        )
        log = 'Some test...\nE2E INTERNAL ERROR\n...\n'

        fail_type, fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.INFRA_FAILURE)
        self.assertEqual(fail_reason, FailedJobReason.E2E_INFRA_FAILURE)

    def test_non_infra_failure_log(self):
        job = ProjectJob(
            MagicMock(),
            attrs={
                "name": "test",
                "id": 618,
                "stage": "test",
                "status": "failed",
                "tag_list": [],
                "allow_failure": False,
                "web_url": "https://sometest.test",
                "retry_summary": ["failed"],
                "failure_reason": "script_failure",
            },
        )
        log = '...E\nTraceback...\n'

        fail_type, _fail_reason = get_job_failure_context(job, log)

        self.assertEqual(fail_type, FailedJobType.JOB_FAILURE)
