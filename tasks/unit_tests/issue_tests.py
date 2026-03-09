import types
import unittest
from unittest.mock import MagicMock, patch

from invoke.context import MockContext, Result

from tasks.issue import (
    DEFAULT_SLACK_CHANNEL,
    GITHUB_SLACK_REVIEW_MAP,
    _get_team_file_counts,
    add_reviewers,
    ask_reviews,
)
from tasks.libs.ciproviders.github_api import get_pr_size


class TestAddReviewers(unittest.TestCase):
    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_dependabot_only(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "InvisibleMan"
        gh_mock.repo.get_pull.return_value = pr_mock
        c = MockContext()
        add_reviewers(c, 1234)
        print_mock.assert_called_once_with("This is not a (dependabot) bump PR, this action should not be run on it.")

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_single_dependency_one_reviewer(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump github.com/redis/go-redis/v9 from 9.1.0 to 9.7.0"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result(
                    """tasks/unit_tests/testdata/add_reviewers/network/fake.go"""
                )
            }
        )
        add_reviewers(c, 1234, owner_file="tasks/unit_tests/testdata/add_reviewers/CODEOWNERS")
        print_mock.assert_not_called()
        pr_mock.create_review_request.assert_called_once_with(team_reviewers=['universal-service-monitoring'])

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_single_dependency_with_folder(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump github.com/redis/go-redis/v9 from 9.1.0 to 9.7.0 in tasks/unit_tests/testdata/add_reviewers/fakeintake"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""tasks/unit_tests/testdata/add_reviewers/network/fake.go
tasks/unit_tests/testdata/add_reviewers/fakeintake/fake.go
""")
            }
        )
        add_reviewers(c, 1234, owner_file="tasks/unit_tests/testdata/add_reviewers/CODEOWNERS")
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'], ['agent-e2e-testing', 'agent-devx']
        )

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_single_dependency_several_reviewers(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump github.com/go-delve/delve from 1.6.0 to 1.7.0"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""tasks/unit_tests/testdata/add_reviewers/fake.go
tasks/unit_tests/testdata/add_reviewers/network/fake.go
tasks/unit_tests/testdata/add_reviewers/debugger/fake.go
""")
            }
        )
        add_reviewers(c, 1234, owner_file="tasks/unit_tests/testdata/add_reviewers/CODEOWNERS")
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            ['universal-service-monitoring', 'debugger', 'agent-devx'],
        )

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_group_dependency(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump the aws-sdk-go-v2 group with 5 updates"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""tasks/unit_tests/testdata/add_reviewers/debugger/fake.go
tasks/unit_tests/testdata/add_reviewers/windows/fake.go
tasks/unit_tests/testdata/add_reviewers/new-e2e/fake.go""")
            }
        )
        add_reviewers(c, 1234, owner_file="tasks/unit_tests/testdata/add_reviewers/CODEOWNERS")
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            [
                'windows-products',
                'debugger',
                'agent-e2e-testing',
            ],
        )

    @patch('builtins.print')
    @patch('tasks.issue.GithubAPI')
    def test_group_dependency_scoped(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.user.login = "dependabot[bot]"
        pr_mock.title = "Bump the aws-sdk-go-v2 group in tasks/unit_tests/testdata/add_reviewers/new-e2e with 5 updates"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance
        c = MockContext(
            run={
                "git ls-files | grep -e \"^.*.go$\"": Result("""tasks/unit_tests/testdata/add_reviewers/debugger/fake.go
tasks/unit_tests/testdata/add_reviewers/windows/fake.go
tasks/unit_tests/testdata/add_reviewers/new-e2e/fake.go""")
            }
        )
        add_reviewers(c, 1234, owner_file="tasks/unit_tests/testdata/add_reviewers/CODEOWNERS")
        print_mock.assert_not_called()
        self.assertCountEqual(
            pr_mock.create_review_request.call_args[1]['team_reviewers'],
            [
                'agent-e2e-testing',
            ],
        )


class TestAskReviews(unittest.TestCase):
    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    @patch('tasks.issue.get_pr_size', new=MagicMock(return_value='medium'))
    @patch('tasks.issue._get_team_file_counts')
    def test_label_with_ask_review(self, file_counts_mock, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "This is a revert"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.get_commits.return_value = [
            MagicMock(commit=MagicMock(message="Revert \"This is a feature\"\n\nThis reverts commit 1234567890")),
        ]
        pr_mock.html_url = "https://github.com/foo/bar/pull/1"
        pr_mock.title = "Nominal PR"

        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        file_counts_mock.return_value = {'team1': 3, 'team2': 2}

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2', 'microwave': 'urlx'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Fill GITHUB_SLACK_REVIEW_MAP to have separate channels for each team
        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/team1'] = 'channel1'
        GITHUB_SLACK_REVIEW_MAP['@datadog/team2'] = 'channel2'

        ask_reviews(MockContext(), 5, "labeled", team_slugs=["team1", "team2"])
        channels = [call.kwargs['channel'] for call in slack_client.chat_postMessage.mock_calls]
        self.assertIn('channel1', channels)
        self.assertIn('channel2', channels)
        self.assertEqual(len(slack_client.chat_postMessage.mock_calls), 2)  # 2 channels
        # Verify personalized messages
        for call in slack_client.chat_postMessage.mock_calls:
            text = call.kwargs['text']
            self.assertIn('`medium` PR', text)
            self.assertIn('file(s) to review', text)

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_no_review_label(self, slack_mock, gh_mock, print_mock):
        """Test that any PR with no-review label is skipped (independent of event type)"""
        pr_mock = MagicMock()
        pr_mock.title = "WIP: Experimental changes"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [
            types.SimpleNamespace(name='ask-review'),
            types.SimpleNamespace(name='no-review'),
        ]
        pr_mock.get_commits.return_value = [MagicMock(commit=MagicMock(message="This is a feature"))]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None
        # ask_reviews returns early on no-review before reading events, but keep events non-empty
        # to match current implementation expectations if the order changes in the future.

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 11, "labeled", team_slugs=["team1"])

        print_mock.assert_any_call("This PR has the no-review label, we don't need to ask for reviews.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_no_review_label_review_requested(self, slack_mock, gh_mock, print_mock):
        """Test that any PR with no-review label is skipped (independent of event type)"""
        pr_mock = MagicMock()
        pr_mock.title = "WIP: Experimental changes"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [
            types.SimpleNamespace(name='no-review'),
        ]
        pr_mock.get_commits.return_value = [MagicMock(commit=MagicMock(message="This is a feature"))]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None
        # ask_reviews returns early on no-review before reading events, but keep events non-empty
        # to match current implementation expectations if the order changes in the future.

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 11, "review_requested", team_slugs=["team1"])

        print_mock.assert_any_call("This PR has the no-review label, we don't need to ask for reviews.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_revert_pr(self, slack_mock, gh_mock, print_mock):
        """Test that any PR with no-review label is skipped (independent of event type)"""
        pr_mock = MagicMock()
        pr_mock.title = "Revert: This is a feature"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [
            types.SimpleNamespace(name='no-review'),
        ]
        pr_mock.get_commits.return_value = [
            MagicMock(commit=MagicMock(message="Revert \"This is a feature\"\n\nThis reverts commit 1234567890"))
        ]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 42, "review_requested", team_slugs=["team1"])

        print_mock.assert_any_call("We don't ask for reviews on revert PRs creation, only on label requests.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_backport(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Backport: fix issue 123"
        pr_mock.base.ref = "7.7.x"
        pr_mock.get_labels.return_value = [MagicMock(name='ask-review')]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None
        # ask_reviews returns early on backport before reading events, but keep events non-empty
        # to match current implementation expectations if the order changes in the future.
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        # team_slugs is required; value doesn't matter because backport returns early
        ask_reviews(MockContext(), 6, "labeled", team_slugs=["team1"])
        print_mock.assert_any_call("We don't ask for reviews on non main target PRs.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {
            'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token',
        },
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    @patch('tasks.issue.get_pr_size', new=MagicMock(return_value='small'))
    @patch('tasks.issue._get_team_file_counts', return_value={'newteam1': 0, 'newteam2': 0})
    def test_default_channel(self, file_counts_mock, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Title"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.get_commits.return_value = [MagicMock(commit=MagicMock(message="This is a feature"))]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = None
        pr_mock.html_url = "http://foo"
        pr_mock.title = "PR with reviewers on DEFAULT_SLACK_CHANNEL"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Clear the mapping so all reviewers fall back to DEFAULT_SLACK_CHANNEL
        GITHUB_SLACK_REVIEW_MAP.clear()
        ask_reviews(MockContext(), 7, "review_requested", team_slugs=["newteam1", "newteam2"])

        # One message because both reviewers fall back to DEFAULT_SLACK_CHANNEL (grouped)
        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        self.assertEqual(kwargs['channel'], DEFAULT_SLACK_CHANNEL)
        self.assertIn("A review channel is missing", kwargs['text'])

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    @patch('tasks.issue.get_pr_size', new=MagicMock(return_value='small'))
    @patch('tasks.issue._get_team_file_counts', return_value={'teamx': 5, 'teamy': 3})
    def test_same_slack_channel(self, file_counts_mock, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Some PR"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.get_commits.return_value = [MagicMock(commit=MagicMock(message="This is a feature"))]
        pr_mock.user.name = "actorlogin"
        pr_mock.html_url = "http://foo"
        pr_mock.title = "Test"
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Map both teams to the same channel
        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/teamx'] = 'chan-shared'
        GITHUB_SLACK_REVIEW_MAP['@datadog/teamy'] = 'chan-shared'

        ask_reviews(MockContext(), 8, "review_requested", team_slugs=["teamx", "teamy"])

        # One message because both teams map to the same channel
        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        self.assertEqual(kwargs['channel'], 'chan-shared')
        self.assertIn("actorlogin", kwargs['text'])
        # Both teams have per-team lines
        self.assertIn("teamx has 5 file(s) to review", kwargs['text'])
        self.assertIn("teamy has 3 file(s) to review", kwargs['text'])
        self.assertIn("primary", kwargs['text'])
        self.assertIn("secondary", kwargs['text'])

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    @patch('tasks.issue.get_pr_size', new=MagicMock(return_value='large'))
    @patch('tasks.issue._get_team_file_counts', return_value={'team1': 10})
    def test_review_request_one_team(self, file_counts_mock, slack_mock, gh_mock, print_mock):
        """Test that when a specific team is requested (review_request event), only that team is notified"""
        pr_mock = MagicMock()
        pr_mock.title = "This is a feature"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.get_commits.return_value = [
            MagicMock(commit=MagicMock(message="This is a feature")),
            MagicMock(commit=MagicMock(message="Revert \"This is a feature\"\n\nThis reverts commit 1234567890")),
        ]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = "actorname"
        pr_mock.html_url = "https://github.com/foo/bar/pull/9"

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Fill GITHUB_SLACK_REVIEW_MAP
        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/team1'] = 'channel1'

        ask_reviews(MockContext(), 9, "review_requested", team_slugs=["team1"])

        # Only one message should be sent (the requested team only)
        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        self.assertEqual(kwargs['channel'], 'channel1')
        self.assertIn("actorname", kwargs['text'])
        self.assertIn("`large` PR", kwargs['text'])
        self.assertIn("team1 has 10 file(s) to review", kwargs['text'])
        self.assertIn("primary", kwargs['text'])

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    @patch('tasks.issue.get_pr_size', new=MagicMock(return_value='small'))
    @patch('tasks.issue._get_team_file_counts', return_value={'team1': 5, 'team2': 0})
    def test_team_zero_owned_files(self, file_counts_mock, slack_mock, gh_mock, print_mock):
        """Test that a team with 0 owned files gets a secondary role"""
        pr_mock = MagicMock()
        pr_mock.title = "Some PR"
        pr_mock.base.ref = "main"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.get_commits.return_value = [MagicMock(commit=MagicMock(message="This is a feature"))]
        pr_mock.user.login = "actorlogin"
        pr_mock.user.name = "actorname"
        pr_mock.html_url = "http://foo"

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/team1'] = 'channel1'
        GITHUB_SLACK_REVIEW_MAP['@datadog/team2'] = 'channel2'

        ask_reviews(MockContext(), 10, "labeled", team_slugs=["team1", "team2"])

        self.assertEqual(len(slack_client.chat_postMessage.mock_calls), 2)
        # Find the message for each channel
        messages = {call.kwargs['channel']: call.kwargs['text'] for call in slack_client.chat_postMessage.mock_calls}
        self.assertIn("team1 has 5 file(s) to review", messages['channel1'])
        self.assertIn("primary", messages['channel1'])
        self.assertIn("team2 has 0 file(s) to review", messages['channel2'])
        self.assertIn("secondary", messages['channel2'])


class TestGetTeamFileCounts(unittest.TestCase):
    def test_basic_counts(self):
        pr_mock = MagicMock()
        pr_mock.get_files.return_value = [
            types.SimpleNamespace(filename='pkg/network/foo.go'),
            types.SimpleNamespace(filename='pkg/network/bar.go'),
            types.SimpleNamespace(filename='pkg/debugger/baz.go'),
        ]
        with patch('tasks.issue.read_owners') as owners_mock:
            co_mock = MagicMock()

            def of_side_effect(filename):
                if filename.startswith('pkg/network'):
                    return [('TEAM', '@DataDog/network-team')]
                if filename.startswith('pkg/debugger'):
                    return [('TEAM', '@DataDog/debugger-team')]
                return []

            co_mock.of.side_effect = of_side_effect
            owners_mock.return_value = co_mock

            counts = _get_team_file_counts(pr_mock, ['network-team', 'debugger-team'])

        self.assertEqual(counts['network-team'], 2)
        self.assertEqual(counts['debugger-team'], 1)

    def test_no_matching_teams(self):
        pr_mock = MagicMock()
        pr_mock.get_files.return_value = [
            types.SimpleNamespace(filename='pkg/other/foo.go'),
        ]
        with patch('tasks.issue.read_owners') as owners_mock:
            co_mock = MagicMock()
            co_mock.of.return_value = [('TEAM', '@DataDog/other-team')]
            owners_mock.return_value = co_mock

            counts = _get_team_file_counts(pr_mock, ['network-team'])

        self.assertEqual(counts['network-team'], 0)


class TestGetPrSize(unittest.TestCase):
    def test_small(self):
        pr = types.SimpleNamespace(changed_files=2, additions=50, deletions=30, review_comments=1)
        self.assertEqual(get_pr_size(pr), 'small')

    def test_medium(self):
        pr = types.SimpleNamespace(changed_files=6, additions=200, deletions=100, review_comments=3)
        self.assertEqual(get_pr_size(pr), 'medium')

    def test_large_by_files(self):
        pr = types.SimpleNamespace(changed_files=15, additions=10, deletions=5, review_comments=0)
        self.assertEqual(get_pr_size(pr), 'large')

    def test_large_by_lines(self):
        pr = types.SimpleNamespace(changed_files=1, additions=600, deletions=100, review_comments=0)
        self.assertEqual(get_pr_size(pr), 'large')

    def test_large_by_comments(self):
        pr = types.SimpleNamespace(changed_files=1, additions=10, deletions=5, review_comments=10)
        self.assertEqual(get_pr_size(pr), 'large')
