from __future__ import annotations

import unittest
from dataclasses import dataclass
from unittest.mock import patch

from invoke.context import Context

import tasks
from tasks.github_tasks import assign_team_label, extract_test_qa_description


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
    CODEOWNERS_FILE = './tasks/unit_tests/testdata/codeowners.txt'

    def make_test(self, changed_files, expected_labels, pr_labels=None, possible_labels=None):
        from tasks.libs.owners.parsing import read_owners

        possible_labels = possible_labels or ['team/team-everything', 'team/team-a', 'team/team-b', 'team/team-doc']

        fake_codeowners = read_owners(TestAssignTeamLabelMock.CODEOWNERS_FILE)

        with (
            patch('tasks.libs.ciproviders.github_api.GithubAPI') as gh_mock,
            patch.object(tasks.github_tasks, 'read_owners') as read_owners_mock,
            patch.object(tasks.github_tasks, '_get_team_labels') as team_labels_mock,
        ):
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
        expected_labels = ['team/team-everything']

        self.make_test(changed_files, expected_labels)

    def test_single_file_multiple_teams(self):
        changed_files = ['README.md']
        expected_labels = ['team/team-a', 'team/team-doc']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_single_team(self):
        changed_files = ['.gitlab/hello/world', '.gitlab/a.py']
        expected_labels = ['team/team-a']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_single_team_best(self):
        # agent-platform has more files than security so only one team will be assigned
        changed_files = ['.gitignore', '.gitlab-ci.yml', '.gitlab/security.yml']
        expected_labels = ['team/team-everything']

        self.make_test(changed_files, expected_labels)

    def test_multiple_files_multiple_teams(self):
        changed_files = ['.gitignore', '.gitlab/security.yml']
        expected_labels = ['team/team-everything', 'team/team-b']

        self.make_test(changed_files, expected_labels)

    def test_skip1(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, pr_labels=['qa/done'])

    def test_skip2(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, pr_labels=['team/team-a'])

    def test_invalid_team_label(self):
        changed_files = ['.gitignore']
        expected_labels = []

        self.make_test(changed_files, expected_labels, possible_labels=['team/team-doc'])


class TestExtractQADescriptionFromPR(unittest.TestCase):
    def test_extract_qa_description(self):
        @dataclass
        class TestCase:
            name: str
            body: str
            expected: str

        testcases: list[TestCase] = [
            TestCase(
                name="Single line",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes
I added one test
### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="I added one test",
            ),
            TestCase(
                name="Multi line",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes
I added one unit test
and one e2e test
### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="""I added one unit test
and one e2e test""",
            ),
            TestCase(
                name="Empty description",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes

### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="",
            ),
            TestCase(
                name="Multiline with subheaders",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes

Here is a test description

#### Step 1

I do this

#### Step 2

Then I do that

##### Substep 2.1

Pay attentions to this

### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="""Here is a test description

#### Step 1

I do this

#### Step 2

Then I do that

##### Substep 2.1

Pay attentions to this""",
            ),
        ]

        for tc in testcases:
            qa_description = extract_test_qa_description(tc.body)
            self.assertEqual(qa_description, tc.expected, f"Test case: {tc.name}")
