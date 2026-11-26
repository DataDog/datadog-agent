import os
import tempfile
import unittest
from unittest.mock import MagicMock, patch

from invoke.context import Context

from tasks.github_tasks import ask_backport_owner as ask_backport_owner_task
from tasks.libs.notify.backport import (
    ADD,
    DISMISS,
    NO_RECIPIENT,
    TIMEOUT,
    BackportRequest,
    BackportSelection,
    ask_backport_owner,
    resolve_owner_email,
    slack_escape,
    valid_branches,
)
from tasks.libs.pipeline.notifications import DEFAULT_SLACK_CHANNEL


def make_request(branches=None) -> BackportRequest:
    return BackportRequest(
        pr_number=42,
        pr_title="fix: <script> & co",
        pr_url="https://github.com/DataDog/datadog-agent/pull/42",
        branches=branches or ["7.85.x", "7.86.x"],
        explanation="Fixes a crash",
        confidence="MEDIUM",
        ttl=7200,
    )


def make_commit(login, email):
    commit = MagicMock()
    commit.author = MagicMock(login=login) if login else None
    commit.commit.author.email = email
    return commit


class TestHelpers(unittest.TestCase):
    def test_valid_branches(self):
        self.assertEqual(valid_branches(" 7.85.x,main,7.86.x, 7.85.x,6.53.x;rm,"), ["7.85.x", "7.86.x"])

    def test_slack_escape(self):
        self.assertEqual(slack_escape("<a> & <b>"), "&lt;a&gt; &amp; &lt;b&gt;")

    def test_section_text_escapes_title(self):
        text = make_request().section_text()
        self.assertIn("fix: &lt;script&gt; &amp; co", text)
        self.assertIn("about 2 hour(s)", text)

    def test_blocks_without_buttons(self):
        self.assertEqual(len(make_request().blocks(None)), 1)

    def test_blocks_keep_apply_and_dismiss_when_all_chosen(self):
        actions = make_request().blocks([])[1]["elements"]
        self.assertEqual([a["action_id"] for a in actions], ["apply", "dismiss"])


class TestResolveOwnerEmail(unittest.TestCase):
    def test_prefers_owner_commit(self):
        pr = MagicMock()
        pr.user.login = "owner"
        pr.get_commits.return_value = [make_commit("owner", "owner@dd.com"), make_commit("other", "other@dd.com")]
        self.assertEqual(resolve_owner_email(pr), "owner@dd.com")

    def test_falls_back_to_last_commit(self):
        pr = MagicMock()
        pr.user.login = "owner"
        pr.get_commits.return_value = [make_commit(None, "a@dd.com"), make_commit("other", "b@dd.com")]
        self.assertEqual(resolve_owner_email(pr), "b@dd.com")

    def test_no_commits(self):
        pr = MagicMock()
        pr.get_commits.return_value = []
        self.assertIsNone(resolve_owner_email(pr))


class TestBackportSelection(unittest.TestCase):
    def setUp(self):
        self.selection = BackportSelection(make_request())
        self.set_buttons = MagicMock()
        self.reply = MagicMock()

    def click(self, action_id, value=""):
        self.selection.handle(action_id, value, self.set_buttons, self.reply)

    def finish(self):
        return self.selection.finish(self.set_buttons, self.reply)

    def test_apply_selection(self):
        self.click("add_label:7.86.x", "7.86.x")
        self.set_buttons.assert_called_with(["7.85.x"])
        self.click("apply", "apply")
        self.assertTrue(self.selection.done.is_set())
        self.assertEqual(self.finish(), ["7.86.x"])
        self.assertEqual(self.selection.outcome, ADD)
        self.set_buttons.assert_called_with(None)

    def test_apply_without_selection(self):
        self.click("apply", "apply")
        self.assertFalse(self.selection.done.is_set())
        self.reply.assert_called_once()

    def test_all_branches_chosen_waits_for_apply(self):
        self.click("add_label:7.85.x", "7.85.x")
        self.click("add_label:7.86.x", "7.86.x")
        self.assertFalse(self.selection.done.is_set())
        self.set_buttons.assert_called_with([])

    def test_dismiss_after_selection_applies_nothing(self):
        self.click("add_label:7.85.x", "7.85.x")
        self.click("dismiss", "dismiss")
        self.assertTrue(self.selection.done.is_set())
        self.assertEqual(self.finish(), [])
        self.assertEqual(self.selection.outcome, DISMISS)

    def test_timeout_with_selection_applies_nothing(self):
        self.click("add_label:7.85.x", "7.85.x")
        self.assertEqual(self.finish(), [])
        self.assertEqual(self.selection.outcome, TIMEOUT)
        self.assertIn("was not applied", self.reply.call_args[0][0])

    def test_unknown_branch_ignored(self):
        self.click("add_label:6.53.x", "6.53.x")
        self.assertEqual(self.selection.chosen, [])
        self.set_buttons.assert_not_called()

    def test_clicks_after_done_ignored(self):
        self.click("dismiss", "dismiss")
        self.click("add_label:7.85.x", "7.85.x")
        self.assertEqual(self.selection.chosen, [])


class TestAskBackportOwner(unittest.TestCase):
    @patch('slack_sdk.WebClient', autospec=True)
    def test_no_slack_user_notifies_devx(self, web_mock):
        from slack_sdk.errors import SlackApiError

        client = web_mock.return_value
        client.users_lookupByEmail.side_effect = SlackApiError("users_not_found", {"ok": False})
        pr = MagicMock()
        pr.user.login = "owner"
        pr.get_commits.return_value = [make_commit("owner", "owner@example.com")]

        self.assertEqual(ask_backport_owner(pr, make_request(), "xoxb", "xapp"), (NO_RECIPIENT, []))
        client.chat_postMessage.assert_called_once()
        self.assertEqual(client.chat_postMessage.call_args.kwargs["channel"], DEFAULT_SLACK_CHANNEL)

    @patch('slack_sdk.WebClient', autospec=True)
    def test_unresolvable_email_notifies_devx(self, web_mock):
        client = web_mock.return_value
        pr = MagicMock()
        pr.get_commits.side_effect = RuntimeError("boom")

        self.assertEqual(ask_backport_owner(pr, make_request(), "xoxb", "xapp"), (NO_RECIPIENT, []))
        self.assertEqual(client.chat_postMessage.call_args.kwargs["channel"], DEFAULT_SLACK_CHANNEL)


class TestAskBackportOwnerTask(unittest.TestCase):
    @patch('tasks.libs.notify.backport.ask_backport_owner', return_value=(ADD, ["7.85.x"]))
    @patch('tasks.libs.ciproviders.github_api.GithubAPI', autospec=True)
    def test_writes_github_output(self, gh_mock, ask_mock):
        pr = MagicMock(number=42, title="title", html_url="url")
        gh_mock.return_value.get_pr.return_value = pr
        with tempfile.NamedTemporaryFile("r", suffix=".txt") as output:
            env = {
                'GITHUB_OUTPUT': output.name,
                'SLACK_DATADOG_AGENT_BOT_TOKEN': 'xoxb',
                'SLACK_BACKPORT_APP_TOKEN': 'xapp',
            }
            with patch.dict(os.environ, env):
                ask_backport_owner_task(Context(), pr_id=42, branches="7.85.x,bogus", confidence="high")
            self.assertEqual(output.read(), "outcome=add\nbranches=7.85.x\n")
        request = ask_mock.call_args[0][1]
        self.assertEqual(request.branches, ["7.85.x"])
        self.assertEqual(request.confidence, "HIGH")

    @patch('tasks.libs.notify.backport.ask_backport_owner')
    def test_no_valid_branch(self, ask_mock):
        ask_backport_owner_task(Context(), pr_id=42, branches="main")
        ask_mock.assert_not_called()
