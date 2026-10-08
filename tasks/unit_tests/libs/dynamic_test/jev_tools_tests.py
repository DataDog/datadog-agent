import json
import re
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from tasks.libs.dynamic_test.jev.jev_client import build_context_state, decide, get_ai_gateway_token
from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite
from tasks.libs.dynamic_test.jev.pr_context import changed_files, fetch_pr_info
from tasks.libs.dynamic_test.jev.pr_summary import summarize_pr
from tasks.libs.dynamic_test.jev.test_discovery import list_suites


def answers(should=0.5, relation="code_under_test", confidence=0.8):
    return {"should_execute": {"noul": should}, "relation": {"choice": relation}, "confidence": {"score": confidence}}


class TestJevTools(unittest.TestCase):
    def test_decision(self):
        self.assertEqual(decide(answers())["decision"], "run")
        self.assertEqual(decide(answers(should=0.01))["decision"], "skip")
        self.assertEqual(decide(answers(relation="unrelated"))["decision"], "skip")
        self.assertEqual(decide(answers(should=0.1))["decision"], "run")

    def test_invalid_model_scores_and_relations(self):
        for value in (float("nan"), float("inf"), None, True, "0.1", -1):
            with (
                self.subTest(value=value),
                # The error names the offending field and carries the raw value
                self.assertRaisesRegex(ValueError, rf"invalid should_execute score: {re.escape(repr(value))}"),
            ):
                decide(answers(should=value))
        with self.assertRaisesRegex(ValueError, "unknown relation: 'typo'"):
            decide(answers(relation="typo"))
        with self.assertRaisesRegex(ValueError, "missing or malformed"):
            decide({})

    def test_legend_scale_scores_are_accepted(self):
        """The model answers on a 0-2 legend scale (the confidence question
        carries a {0,1,2} legend): scores at or above 0 are accepted, so a
        1.2 confidence does not fail the decision open."""
        for value in (0, 0.5, 1, 1.2, 2):
            with self.subTest(value=value):
                decide(answers(confidence=value))
        self.assertEqual(decide(answers(should=2))["decision"], "run")

    @patch("tasks.libs.dynamic_test.jev.jev_client.subprocess.run")
    def test_token_cmd_is_shell_split(self, run):
        run.return_value = SimpleNamespace(returncode=0, stdout="token\n", stderr="")
        with patch.dict("os.environ", {}, clear=True):
            self.assertEqual(get_ai_gateway_token(token_cmd='tool --name "two words"'), "token")
        run.assert_called_once_with(["tool", "--name", "two words"], capture_output=True, text=True, timeout=60)

    @patch("tasks.libs.dynamic_test.jev.jev_client.subprocess.run")
    def test_auth_fallback_runs_authanywhere(self, run):
        run.return_value = SimpleNamespace(returncode=0, stdout="token", stderr="")
        with patch.dict("os.environ", {}, clear=True):
            self.assertEqual(get_ai_gateway_token(dc="us1.ddbuild.io"), "token")
            # The explicit token override short-circuits before it
            self.assertEqual(get_ai_gateway_token(token="override"), "override")
        run.assert_called_once_with(
            ["authanywhere", "--audience", "rapid-ai-platform", "--raw", "--dc", "us1.ddbuild.io"],
            capture_output=True,
            text=True,
            timeout=60,
        )

    @patch("tasks.libs.dynamic_test.jev.pr_context.GithubAPI")
    @patch("tasks.libs.dynamic_test.jev.pr_context.git", return_value="feature/nested")
    def test_pr_lookup_by_branch_uses_the_shared_github_api(self, _, github_api):
        github_api.return_value.get_pr_for_branch.return_value = iter(
            [SimpleNamespace(number=12, title="Title", body="Description")]
        )
        with patch.dict("os.environ", {"GITHUB_TOKEN": "fake"}, clear=True):
            info = fetch_pr_info("main", None)
        self.assertEqual(info["number"], 12)
        self.assertEqual(info["title"], "Title")
        self.assertEqual(info["description"], "Description")
        github_api.return_value.get_pr_for_branch.assert_called_once_with(head_branch_name="feature/nested")

    @patch("tasks.libs.dynamic_test.jev.pr_context.GithubAPI")
    @patch("tasks.libs.dynamic_test.jev.pr_context.git", return_value="feature/nested")
    def test_pr_lookup_without_token_or_without_pr_degrades_gracefully(self, _, github_api):
        with patch.dict("os.environ", {}, clear=True):
            info = fetch_pr_info("main", None)
        self.assertEqual(info, {"branch": "feature/nested", "title": "", "description": ""})
        github_api.assert_not_called()
        github_api.return_value.get_pr_for_branch.return_value = iter([])
        with patch.dict("os.environ", {"GITHUB_TOKEN": "fake"}):
            info = fetch_pr_info("main", None)
        self.assertNotIn("number", info)
        self.assertEqual(info["title"], "")

    @patch("tasks.libs.dynamic_test.jev.pr_context.git")
    def test_ddci_file_list_uses_the_local_merge_base_not_ddci_base_commit(self, git):
        """DDCI's base_commit is the base branch TIP (GitHub PR base.sha), not
        the merge base: a diff against it includes all of main's changes since
        the fork point."""
        git.side_effect = ["main", "0c339c19"]  # rev-parse candidate, merge-base
        files, merge_base = changed_files("main", {"changed_files": [("a.go", "added")], "base_commit": "ca52e138d6d1"})
        self.assertEqual(files, [("a.go", "added")])
        self.assertEqual(merge_base, "0c339c19")
        git.assert_called_with("merge-base", "HEAD", "main")

    @patch("tasks.libs.dynamic_test.jev.pr_context.git")
    def test_changed_files_git_fallback(self, git):
        git.side_effect = ["main", "0c339c19", "a.go\nb.go\n"]  # rev-parse, merge-base, diff
        files, merge_base = changed_files("main", None)
        self.assertEqual(files, [("a.go", ""), ("b.go", "")])
        self.assertEqual(merge_base, "0c339c19")

    def test_discovery_ignores_helpers(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "helpers.go").write_text("package test\nfunc TestHelper(t *testing.T) {}\n")
            Path(directory, "suite_test.go").write_text(
                "package test\nfunc TestSuite(t *testing.T) {}\nfunc TestHelper(x string) {}\n"
                "func (s *Suite) TestMethod() {}\n"
            )
            self.assertEqual([entry[0] for entry in list_suites(directory)], ["TestSuite"])

    def test_selector_fails_open_on_malformed_response_and_duplicate_names(self):
        module = "tasks.libs.dynamic_test.jev.jev_e2e_selector"
        with tempfile.TemporaryDirectory() as directory:
            output = str(Path(directory, "decisions.json"))
            with (
                patch(f"{module}.os.path.isdir", return_value=True),
                patch(f"{module}.fetch_ddci_metadata", return_value=None),
                patch(f"{module}.fetch_pr_info", return_value={}),
                patch(f"{module}.changed_files", return_value=([], "base")),
                patch(f"{module}.pr_diff", return_value=""),
                patch(f"{module}.suite_definition", return_value=("", "")),
                patch(f"{module}.get_ai_gateway_token", return_value="fake"),
                patch(f"{module}.summarize_pr", return_value="a summary"),
                patch(f"{module}.gitlab_section") as section,
                patch(f"{module}._printed_contexts", set()),
                patch(f"{module}._pr_summaries", {}),
                patch(
                    f"{module}.list_suites",
                    return_value=[
                        ("TestDuplicate", "one_test.go", "code"),
                        ("TestDuplicate", "two_test.go", "code"),
                        ("TestUnavailable", "three_test.go", "code"),
                    ],
                ),
                patch(
                    f"{module}.ask_jev",
                    side_effect=[{"answers": answers(0.0, "unrelated")}, {"answers": {}}, RuntimeError("offline")],
                ),
            ):
                select_suite("fleet", workers=1, output=output)
            summary = json.loads(Path(output).read_text())
            self.assertEqual(summary["run"], ["TestDuplicate", "TestUnavailable"])
            self.assertEqual(summary["skip"], [])
            self.assertIn("error", summary["decisions"][1])
            # The context (state without the test code) is printed ONCE for the
            # whole suite - not once per Jev call
            section.assert_called_once()

    def test_pr_summary_replaces_the_diff_in_the_jev_state(self):
        """The generated summary is computed once per run, sent instead of the
        raw diff in every per-test Jev state, and a failed generation falls
        back to the diff."""

        def summarize_mock(side_effect):
            def call(*args, **kwargs):
                if isinstance(side_effect, Exception):
                    raise side_effect
                return side_effect

            return call

        module = "tasks.libs.dynamic_test.jev.jev_e2e_selector"
        states = []
        with tempfile.TemporaryDirectory() as directory:
            for side_effect in ("The PR adds a new config field.", RuntimeError("gateway down")):
                states.clear()
                with (
                    patch(f"{module}.os.path.isdir", return_value=True),
                    patch(f"{module}.fetch_ddci_metadata", return_value=None),
                    patch(f"{module}.fetch_pr_info", return_value={"title": "t", "description": "d"}),
                    patch(f"{module}.changed_files", return_value=([("a.go", "modified")], "base")),
                    patch(f"{module}.pr_diff", return_value="```diff\n+ a change\n```"),
                    patch(f"{module}.suite_definition", return_value=("", "")),
                    patch(f"{module}.get_ai_gateway_token", return_value="fake"),
                    patch(f"{module}.summarize_pr", side_effect=summarize_mock(side_effect)),
                    patch(f"{module}.gitlab_section"),
                    patch(f"{module}._printed_contexts", set()),
                    patch(f"{module}._pr_summaries", {}),
                    patch(
                        f"{module}.list_suites",
                        return_value=[
                            ("TestOne", "one_test.go", "code"),
                            ("TestTwo", "two_test.go", "code"),
                        ],
                    ),
                    patch(
                        f"{module}.ask_jev",
                        side_effect=lambda token, state, **k: states.append(state) or {"answers": answers()},
                    ),
                ):
                    select_suite("fleet", workers=1, output=str(Path(directory, "decisions.json")))
                self.assertEqual(len(states), 2)
                if isinstance(side_effect, str):
                    for state in states:
                        self.assertIn("The PR adds a new config field.", state)
                        self.assertIn("LLM summary of the changes in this PR", state)
                        self.assertNotIn("```diff", state)  # the raw diff is replaced
                else:
                    for state in states:  # failed generation: the raw diff is sent
                        self.assertIn("```diff\n+ a change\n```", state)

    def test_build_context_state_summary_and_diff_sections(self):
        kwargs = {
            "suite": "fleet",
            "team": "fleet",
            "pr": {"title": "t", "description": "d"},
            "files": [("a.go", "modified")],
            "merge_base": "0c339c19",
            "diff": "```diff\n+ a change\n```",
        }
        with_diff = build_context_state(**kwargs)
        self.assertIn("## Full PR diff", with_diff)
        self.assertNotIn("LLM summary", with_diff)
        summarized = build_context_state(pr_summary="summary text", **kwargs)
        self.assertIn("## LLM summary of the changes in this PR", summarized)
        self.assertIn("summary text", summarized)
        self.assertNotIn("## Full PR diff", summarized)

    @patch("tasks.libs.dynamic_test.jev.pr_summary.urllib.request.urlopen")
    def test_summarize_pr_calls_the_ai_gateway_chat_completions(self, urlopen):
        """The summary call goes to the gateway's OpenAI-compatible endpoint
        with the same auth and headers as the Jev (System One) calls."""
        body = json.dumps({"choices": [{"message": {"content": "  The PR fixes a flaky test.  "}}]}).encode()
        urlopen.return_value.__enter__.return_value = urlopen.return_value
        urlopen.return_value.__exit__.return_value = False
        urlopen.return_value.read.return_value = body
        summary = summarize_pr(
            "token",
            {"title": "Fix flake", "description": "d"},
            [("a.go", "modified")],
            "0c339c19",
            "diff text",
            model="gpt-4o-mini",
            dc="us1.ddbuild.io",
        )
        self.assertEqual(summary, "The PR fixes a flaky test.")
        req = urlopen.call_args[0][0]
        self.assertEqual(req.full_url, "https://ai-gateway.us1.ddbuild.io/v1/chat/completions")
        self.assertEqual(req.get_header("Authorization"), "Bearer token")
        body = json.loads(req.data)
        self.assertEqual(body["model"], "gpt-4o-mini")
        self.assertIn("Fix flake", body["messages"][1]["content"])
        self.assertIn("diff text", body["messages"][1]["content"])
