import unittest
from unittest.mock import patch

from invoke.context import Context

import tasks
from tasks.github_tasks import assign_team_label


class GithubAPIMock:
    def __init__(self, pr_labels: list[str], pr_files: list[str]) -> None:
        self.pr_labels = pr_labels
        self.pr_files = pr_files
        self.assigned_labels = []

    def get_pr_files(self, *_args, **_kwargs):
        return self.pr_files

    def get_pr_labels(self, *_args, **_kwargs):
        return self.pr_labels

    def add_pr_label(self, _pr_id, label_name):
        self.assigned_labels.append(label_name)


class TestAssignTeamLabelMock(unittest.TestCase):
    CODEOWNERS_FILE = './tasks/unit-tests/testdata/codeowners.txt'

    def make_test(self, changed_files, expected_labels):
        from tasks.libs.pipeline_notifications import read_owners

        fake_codeowners = read_owners(TestAssignTeamLabelMock.CODEOWNERS_FILE)

        with patch('tasks.libs.common.github_api.GithubAPI') as gh_mock, patch.object(
            tasks.github_tasks, 'read_owners'
        ) as read_owners_mock:
            gh = GithubAPIMock(
                [],
                changed_files,
            )
            gh_mock.side_effect = [gh]
            read_owners_mock.return_value = fake_codeowners

            assign_team_label(Context(), -1)

            self.assertEqual(sorted(gh.assigned_labels), sorted(expected_labels))

    def test_no_match(self):
        changed_files = ['idonotexist']
        expected_labels = []

        self.make_test(changed_files, expected_labels)

    def test_no_file(self):
        changed_files = []
        expected_labels = []

        self.make_test(changed_files, expected_labels)

    def test_single_file_single_team(self):
        changed_files = ['.gitignore']
        expected_labels = ['team/agent-platform']

        self.make_test(changed_files, expected_labels)

    def test_single_file_multiple_teams(self):
        changed_files = ['README.md']
        expected_labels = ['team/agent-platform', 'team/documentation']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_single_team(self):
        changed_files = ['.gitignore', '.gitlab/a.py']
        expected_labels = ['team/agent-platform']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_single_team_best(self):
        # agent-platform has more files than security so only one team will be assigned
        changed_files = ['.gitignore', '.gitlab-ci.yml', '.gitlab/security.yml']
        expected_labels = ['team/agent-platform']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_multiple_teams(self):
        changed_files = ['.gitignore', '.gitlab/security.yml']
        expected_labels = ['team/agent-platform', 'team/agent-security']

        self.make_test(changed_files, expected_labels)
