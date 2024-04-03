import unittest

from tasks.libs import pipeline_notifications


class TestLoadAndValidate(unittest.TestCase):
    def test_files_loaded_correctly(self):
        # Assert that a couple of expected entries are there, including one that uses DEFAULT_JIRA_PROJECT
        self.assertEqual(pipeline_notifications.GITHUB_JIRA_MAP['@datadog/agent-all'], "AGNTR")
        self.assertEqual(pipeline_notifications.GITHUB_JIRA_MAP['@datadog/agent-ci-experience'], "ACIX")

        # Assert that a couple of expected entries are there, including one that uses DEFAULT_SLACK_PROJECT
        self.assertEqual(pipeline_notifications.GITHUB_SLACK_MAP['@datadog/agent-all'], "#datadog-agent-pipelines")
        self.assertEqual(
            pipeline_notifications.GITHUB_SLACK_MAP['@datadog/agent-ci-experience'], "#agent-developer-experience"
        )
