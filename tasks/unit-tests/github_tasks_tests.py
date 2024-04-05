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

    def make_test(self, changed_files, expected_labels, pr_labels=None, possible_labels=None):
        from tasks.libs.owners.parsing import read_owners

        possible_labels = possible_labels or ['team/agent-platform', 'team/documentation', 'team/agent-security']

        fake_codeowners = read_owners(TestAssignTeamLabelMock.CODEOWNERS_FILE)

        with patch('tasks.libs.ciproviders.github_api.GithubAPI') as gh_mock, patch.object(
            tasks.github_tasks, 'read_owners'
        ) as read_owners_mock, patch.object(tasks.github_tasks, '_get_team_labels') as team_labels_mock:
            gh = GithubAPIMock(
                pr_labels or [],
                changed_files,
            )
            gh_mock.return_value = gh
            read_owners_mock.return_value = fake_codeowners
            team_labels_mock.return_value = possible_labels

            assign_team_label(Context())

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

    def test_skip1(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, pr_labels=['qa/done'])

    def test_skip2(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, pr_labels=['team/agent-platform'])

    def test_invalid_team_label(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, possible_labels=['team/documentation'])
