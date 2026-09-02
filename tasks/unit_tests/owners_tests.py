import os
import tempfile
import unittest
from unittest import mock

from tasks.owners import (
    _team_slugs,
    smp_pr_context_impl,
)

# CODEOWNERS with repo-relative patterns, for the involved-teams computation.
CODEOWNERS = """\
/test/regression/ @DataDog/single-machine-performance
/test/regression/logs/ @DataDog/agent-log-pipelines
"""


class TestTeamSlugs(unittest.TestCase):
    def test_normalizes_and_filters_to_teams(self):
        self.assertEqual(
            _team_slugs(["@DataDog/Agent-Log-Pipelines", "@DataDog/single-machine-performance"]),
            ["agent-log-pipelines", "single-machine-performance"],
        )

    def test_drops_non_datadog_owners(self):
        self.assertEqual(_team_slugs(["@someuser", "@DataDog/team-a"]), ["team-a"])


class TestSmpPrContext(unittest.TestCase):
    """smp_pr_context_impl returns (labels_csv, involved_teams_csv): labels from the PR, involved teams
    from CODEOWNERS ownership of the PR's changed files. These are the only inputs SMP selection needs."""

    def setUp(self):
        fd, self.owners_file = tempfile.mkstemp()
        with os.fdopen(fd, "w") as f:
            f.write(CODEOWNERS)

    def tearDown(self):
        os.remove(self.owners_file)

    @mock.patch("tasks.libs.ciproviders.github_api.GithubAPI")
    def test_returns_labels_and_involved_teams(self, mock_gh_cls):
        gh = mock_gh_cls.return_value
        gh.get_pr_for_branch.return_value = [mock.Mock(number=42)]
        gh.get_pr_labels.return_value = ["smp/logs/syslog", "team/agent-log-pipelines"]
        gh.get_pr_files.return_value = [
            "test/regression/logs/general/cases/exp/experiment.yaml",  # -> agent-log-pipelines
            "tasks/owners.py",  # -> not owned in this CODEOWNERS -> no team
        ]
        labels, involved = smp_pr_context_impl("mybranch", "DataDog/datadog-agent", self.owners_file)
        self.assertEqual(labels, "smp/logs/syslog,team/agent-log-pipelines")
        self.assertEqual(involved, "agent-log-pipelines")
        gh.get_pr_for_branch.assert_called_once_with(head_branch_name="mybranch")

    @mock.patch("tasks.libs.ciproviders.github_api.GithubAPI")
    def test_no_open_pr_returns_empty(self, mock_gh_cls):
        mock_gh_cls.return_value.get_pr_for_branch.return_value = []
        labels, involved = smp_pr_context_impl("mybranch", "DataDog/datadog-agent", self.owners_file)
        self.assertEqual(labels, "")
        self.assertEqual(involved, "")


if __name__ == "__main__":
    unittest.main()
