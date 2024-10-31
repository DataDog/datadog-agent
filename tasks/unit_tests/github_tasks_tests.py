from __future__ import annotations

import unittest
from dataclasses import dataclass
from unittest.mock import Mock, patch

from invoke.context import Context

import tasks
from tasks.github_tasks import Exit, assign_team_label, extract_test_qa_description, pr_merge_dd_event_sender


class GithubAPIMock:
    from github.PullRequest import PullRequest

    def __init__(self, pr_labels: list[str] = None, pr_files: list[str] = None, pr: PullRequest = None) -> None:
        self.pr_labels = pr_labels or []
        self.pr_files = pr_files or []
        self.assigned_labels = []
        self.pr = pr

    def get_pr_files(self, *_args, **_kwargs):
        return self.pr_files

    def get_pr_labels(self, *_args, **_kwargs):
        return self.pr_labels

    def add_pr_label(self, _pr_id, label_name):
        self.assigned_labels.append(label_name)

    def get_pr(self, _pr_id):
        return self.pr


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
                pr_labels,
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
        class TestExtractBodyCase:
            name: str
            body: str
            expected: str

        testcases: list[TestExtractBodyCase] = [
            TestExtractBodyCase(
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
            TestExtractBodyCase(
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
            TestExtractBodyCase(
                name="Empty description",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes

### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="",
            ),
            TestExtractBodyCase(
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
            TestExtractBodyCase(
                name="Single line with special characters",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes

Here is a test description with special characters: `~!@#$,%^&*()_-+={[]}|\\:;\"'<>.?/

### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="Here is a test description with special characters: `~!@#$,%^&*()_-+={[]}|\\:;\"'<>.?/",
            ),
            TestExtractBodyCase(
                name="Missing test description header",
                body="""### What does this PR do?

### Motivation

### Possible Drawbacks / Trade-offs

### Additional Notes
""",
                expected="",
            ),
            TestExtractBodyCase(
                name="Missing next section header",
                body="""### What does this PR do?

### Motivation

### Describe how to test/QA your changes
Here is how to test this PR
""",
                expected="Here is how to test this PR",
            ),
        ]

        for tc in testcases:
            qa_description = extract_test_qa_description(tc.body)
            self.assertEqual(qa_description, tc.expected, f"Test case: {tc.name}")


class TestPRMergeDDEVentSender(unittest.TestCase):
    def test_send_pr_merge_event(self):
        @dataclass
        class TestSendPRMergeEventCase:
            name: str
            labels: list[str]
            merged: bool
            number: int
            login: str
            body: str
            ref: str
            repo_full_name: str
            expected_text: str
            expected_tags: list[str]
            dry_run: bool = False
            expected_error: str = None

        testcases: list[TestSendPRMergeEventCase] = [
            TestSendPRMergeEventCase(
                name="PR merged, qa/done label, with description",
                labels=["qa/done"],
                merged=True,
                number=123,
                login="testuser",
                body="""
### Describe how to test/QA your changes
This is covered by unit tests
### Possible Drawbacks / Trade-offs""",
                ref="main",
                repo_full_name="testorg/testrepo",
                expected_text="PR #123 merged to main at testorg/testrepo by testuser with QA description [This is covered by unit tests]",
                expected_tags=[
                    "repo:testorg/testrepo",
                    "pr_id:123",
                    "author:testuser",
                    "qa_label:qa/done",
                ],
                expected_error=None,
            ),
            TestSendPRMergeEventCase(
                name="PR not merged",
                labels=[],
                merged=False,
                number=123,
                login="testuser",
                body="",
                ref="main",
                repo_full_name="testorg/testrepo",
                expected_text="",
                expected_tags=[],
                expected_error="PR #123 is not merged yet",
            ),
            TestSendPRMergeEventCase(
                name="PR merged, qa/no-code-change label, no description, team label",
                labels=["qa/no-code-change"],
                merged=True,
                number=123,
                login="testuser",
                body="""
### Describe how to test/QA your changes

### Possible Drawbacks / Trade-offs""",
                ref="main",
                repo_full_name="testorg/testrepo",
                expected_text="PR #123 merged to main at testorg/testrepo by testuser with QA description []",
                expected_tags=[
                    "repo:testorg/testrepo",
                    "pr_id:123",
                    "author:testuser",
                    "qa_label:qa/no-code-change",
                    "qa_description:missing",
                ],
                expected_error=None,
            ),
            TestSendPRMergeEventCase(
                name="PR merged, no qa label, with description",
                labels=["team/team-a"],
                merged=True,
                number=123,
                login="testuser",
                body="""
### Describe how to test/QA your changes
You should do
#### Step 1
Create an ubuntu VM
#### Step 2
Install the agent
#### Step 3
Send some logs
#### Step 4
Check the logs in DD
### Possible Drawbacks / Trade-offs""",
                ref="main",
                repo_full_name="testorg/testrepo",
                expected_text="""PR #123 merged to main at testorg/testrepo by testuser with QA description [You should do
#### Step 1
Create an ubuntu VM
#### Step 2
Install the agent
#### Step 3
Send some logs
#### Step 4
Check the logs in DD]""",
                expected_tags=[
                    "repo:testorg/testrepo",
                    "pr_id:123",
                    "author:testuser",
                    "qa_label:missing",
                    "team:team-a",
                ],
                expected_error=None,
            ),
        ]

        for tc in testcases:
            with (
                patch('tasks.libs.ciproviders.github_api.GithubAPI') as gh_mock,
                patch('tasks.github_tasks.send_event') as send_event_mock,
            ):
                from github.NamedUser import NamedUser
                from github.PullRequestPart import PullRequestPart
                from github.Repository import Repository

                repo_mock = unittest.mock.Mock(spec=Repository)
                repo_mock.full_name = tc.repo_full_name

                base_mock = unittest.mock.Mock(spec=PullRequestPart)
                base_mock.ref = tc.ref
                base_mock.repo = repo_mock

                user_mock = Mock(spec=NamedUser)
                user_mock.login = tc.login

                pr_mock = unittest.mock.Mock(spec=GithubAPIMock.PullRequest)
                pr_mock.number = tc.number
                pr_mock.user = user_mock
                pr_mock.body = tc.body
                pr_mock.base = base_mock
                pr_mock.merged = tc.merged

                gh = GithubAPIMock(
                    pr_labels=tc.labels,
                    pr=pr_mock,
                )
                gh_mock.return_value = gh

                send_event_mock.return_value = None

                # with self.assertRaises(Exit) as exit_mock:
                try:
                    pr_merge_dd_event_sender(Context(), pr_id=tc.number, dry_run=tc.dry_run)
                except Exit as exception:
                    if tc.expected_error:
                        send_event_mock.assert_not_called()
                        self.assertEqual(exception.code, 1, f"Test case: {tc.name}")
                        self.assertEqual(exception.message, tc.expected_error, f"Test case: {tc.name}")
                        continue
                    self.fail(f"Test case: {tc.name} should not have raised an error")

                if tc.expected_error:
                    self.fail(f"Test case: {tc.name} should have raised an error")

                send_event_mock.assert_called_once_with(title="PR merged", text=tc.expected_text, tags=tc.expected_tags)
