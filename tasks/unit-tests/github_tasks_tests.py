import unittest

from tasks.github_tasks import _get_teams


class TestAssignTeamLabel(unittest.TestCase):
    CODEOWNERS_FILE = './tasks/unit-tests/testdata/codeowners.txt'

    def test_no_match(self):
        changed_files = ['idonotexist']
        expected_teams = []

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_no_file(self):
        changed_files = ['idonotexist']
        expected_teams = []

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_single_file_single_team(self):
        changed_files = ['.gitignore']
        expected_teams = ['@DataDog/agent-platform']

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_single_file_multiple_teams(self):
        changed_files = ['README.md']
        expected_teams = ['@DataDog/agent-platform', '@DataDog/documentation']

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_multiple_files_single_team(self):
        changed_files = ['.gitignore', '.gitlab/a.py']
        expected_teams = ['@DataDog/agent-platform']

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_multiple_files_single_team_best(self):
        # agent-platform has more files than security so only one team will be assigned
        changed_files = ['.gitignore', '.gitlab-ci.yml', '.gitlab/security.yml']
        expected_teams = ['@DataDog/agent-platform']

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))

    def test_multiple_files_multiple_teams(self):
        changed_files = ['.gitignore', '.gitlab/security.yml']
        expected_teams = ['@DataDog/agent-platform', '@DataDog/agent-security']

        teams = _get_teams(changed_files, TestAssignTeamLabel.CODEOWNERS_FILE)

        self.assertEqual(sorted(teams), sorted(expected_teams))
