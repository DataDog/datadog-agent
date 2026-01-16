import types
import unittest
from unittest.mock import MagicMock, patch

from invoke.context import MockContext, Result

from tasks.issue import DEFAULT_SLACK_CHANNEL, GITHUB_SLACK_REVIEW_MAP, add_reviewers, ask_reviews


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
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_nominal(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "This is a feature"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.user.login = "someuser"
        pr_mock.html_url = "https://github.com/foo/bar/pull/1"
        pr_mock.title = "Nominal PR"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="labeled",
                label=types.SimpleNamespace(name="ask-review"),
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={},
            )
        ]

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2', 'microwave': 'urlx'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Fill GITHUB_SLACK_REVIEW_MAP to have separate channels for each team
        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/team1'] = 'channel1'
        GITHUB_SLACK_REVIEW_MAP['@datadog/team2'] = 'channel2'

        ask_reviews(MockContext(), 5)
        channels = [call.kwargs['channel'] for call in slack_client.chat_postMessage.mock_calls]
        self.assertIn('channel1', channels)
        self.assertIn('channel2', channels)
        self.assertEqual(len(slack_client.chat_postMessage.mock_calls), 2)  # 2 teams

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    def test_ask_reviews_backport(self, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Backport: fix issue 123"
        pr_mock.get_labels.return_value = [MagicMock(name='ask-review')]
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 6)
        print_mock.assert_any_call("This is a backport PR, we don't need to ask for reviews.")

    @patch('builtins.print')
    @patch(
        'os.environ',
        {
            'PR_REQUESTED_TEAMS': '[{"slug": "newteam1"}, {"slug": "newteam2"}]',
            'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token',
        },
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_default_channel(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Title"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.user.login = "frank"
        pr_mock.html_url = "http://foo"
        pr_mock.title = "PR with reviewers on DEFAULT_SLACK_CHANNEL"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="labeled",
                label=types.SimpleNamespace(name="ask-review"),
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={},
            )
        ]
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Remove mapping so both go to default
        from tasks.issue import GITHUB_SLACK_REVIEW_MAP

        GITHUB_SLACK_REVIEW_MAP.clear()

        ask_reviews(MockContext(), 7)

        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        assert kwargs['channel'] == DEFAULT_SLACK_CHANNEL
        assert "A review channel is missing" in kwargs['text']

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "teamx"}, {"slug": "teamy"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_same_slack_channel(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Some PR"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.user.login = "integrationbot"
        pr_mock.html_url = "http://foo"
        pr_mock.title = "Test"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="labeled",
                label=types.SimpleNamespace(name="ask-review"),
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={},
            )
        ]
        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Map both teams to the same channel
        GITHUB_SLACK_REVIEW_MAP['@datadog/teamx'] = 'chan-shared'
        GITHUB_SLACK_REVIEW_MAP['@datadog/teamy'] = 'chan-shared'

        ask_reviews(MockContext(), 8)

        # Only one message for the shared channel, mentioning both reviewers
        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        self.assertEqual(kwargs['channel'], 'chan-shared')

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_from_review_request(self, slack_mock, gh_mock, print_mock):
        """Test that when a specific team is requested (review_request event), only that team is notified"""
        pr_mock = MagicMock()
        pr_mock.title = "Feature PR"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.user.login = "someuser"
        pr_mock.html_url = "https://github.com/foo/bar/pull/9"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="review_requested",
                label=None,
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={"requested_team": {"slug": "team42"}},
            )
        ]

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        emoji_list = {'emoji': {'wave': 'url1', 'waves': 'url2'}}
        slack_client = MagicMock()
        slack_client.emoji_list.return_value = types.SimpleNamespace(data=emoji_list)
        slack_mock.return_value = slack_client

        # Fill GITHUB_SLACK_REVIEW_MAP
        GITHUB_SLACK_REVIEW_MAP.clear()
        GITHUB_SLACK_REVIEW_MAP['@datadog/team42'] = 'channel42'
        GITHUB_SLACK_REVIEW_MAP['@datadog/team2'] = 'channel2'

        ask_reviews(MockContext(), 9)

        # Only one message should be sent (the requested team only)
        slack_client.chat_postMessage.assert_called_once()
        args, kwargs = slack_client.chat_postMessage.call_args
        self.assertEqual(kwargs['channel'], 'channel42')

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_unique_reviewer_request_ignored(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Feature PR"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='ask-review')]
        pr_mock.user.login = "someuser"
        pr_mock.html_url = "https://github.com/foo/bar/pull/99"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="review_requested",
                label=None,
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={"requested_reviewer": {"login": "some-reviewer"}},
            )
        ]

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 99)

        print_mock.assert_any_call("This is a unique reviewer request, we ignore it.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_labeled_but_no_ask_review_label_ignored(self, slack_mock, gh_mock, print_mock):
        pr_mock = MagicMock()
        pr_mock.title = "Feature PR"
        pr_mock.get_labels.return_value = [types.SimpleNamespace(name='needs-review')]
        pr_mock.user.login = "someuser"
        pr_mock.html_url = "https://github.com/foo/bar/pull/100"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="labeled",
                label=types.SimpleNamespace(name="needs-review"),
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={},
            )
        ]

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 100)

        print_mock.assert_any_call("This is a labeled event, but the label is not 'ask-review', we ignore it.")
        slack_mock.assert_not_called()

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}, {"slug": "team2"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    @patch('slack_sdk.WebClient')
    def test_ask_reviews_from_revew_requested(self, slack_mock, gh_mock, print_mock):
        """Test that when PR is marked as review_requested, requested team is notified"""
        pr_mock = MagicMock()
        pr_mock.title = "Draft PR now ready"
        pr_mock.user.login = "developer"
        pr_mock.html_url = "https://github.com/foo/bar/pull/10"
        pr_mock.get_issue_events.return_value = [
            types.SimpleNamespace(
                event="review_requested",
                label=None,
                actor=types.SimpleNamespace(name="actor", login="actorlogin"),
                raw_data={},
            )
        ]

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
        GITHUB_SLACK_REVIEW_MAP['@datadog/team2'] = 'channel2'

        ask_reviews(MockContext(), 10)

        # Both teams should be notified
        self.assertEqual(len(slack_client.chat_postMessage.mock_calls), 2)
        channels = [call.kwargs['channel'] for call in slack_client.chat_postMessage.mock_calls]
        self.assertIn('channel1', channels)
        self.assertIn('channel2', channels)

    @patch('builtins.print')
    @patch(
        'os.environ',
        {'PR_REQUESTED_TEAMS': '[{"slug": "team1"}]', 'SLACK_DATADOG_AGENT_BOT_TOKEN': 'fake-token'},
    )
    @patch('tasks.issue.GithubAPI')
    def test_ask_reviews_with_no_review_label(self, gh_mock, print_mock):
        """Test that PR with no-review label is skipped"""
        pr_mock = MagicMock()
        pr_mock.title = "WIP: Experimental changes"
        pr_mock.get_labels.return_value = [
            types.SimpleNamespace(name='ask-review'),
            types.SimpleNamespace(name='no-review'),
        ]

        gh_instance = MagicMock()
        gh_instance.repo.get_pull.return_value = pr_mock
        gh_mock.return_value = gh_instance

        ask_reviews(MockContext(), 11)

        print_mock.assert_any_call("This PR has the no-review label, we don't need to ask for reviews.")
