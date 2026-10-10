import json
import re
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from tasks.libs.dynamic_test.jev.jev_client import decide, get_ai_gateway_token
from tasks.libs.dynamic_test.jev.jev_e2e_selector import DEFAULT_SUMMARY_MODEL, generate_pr_summary, select_suite
from tasks.libs.dynamic_test.jev.pr_context import changed_files, fetch_pr_info
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
                patch(f"{module}.gitlab_section") as section,
                patch(f"{module}._printed_contexts", set()),
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

    def test_injected_llm_summary_replaces_the_diff_without_any_llm_call(self):
        module = "tasks.libs.dynamic_test.jev.jev_e2e_selector"

        def run(**kwargs):
            with (
                patch(f"{module}.os.path.isdir", return_value=True),
                patch(f"{module}.fetch_ddci_metadata", return_value=None),
                patch(f"{module}.fetch_pr_info", return_value={}),
                patch(f"{module}.changed_files", return_value=([], "base")),
                patch(f"{module}.pr_diff", return_value="RAW DIFF"),
                patch(f"{module}.suite_definition", return_value=("", "")),
                patch(f"{module}.get_ai_gateway_token", return_value="fake"),
                patch(f"{module}.gitlab_section"),
                patch(f"{module}._printed_contexts", set()),
                patch(f"{module}.list_suites", return_value=[("TestA", "a_test.go", "code")]),
                patch(f"{module}.summarize_pr") as summarize,
                patch(f"{module}.ask_jev", return_value={"answers": answers()}) as ask,
            ):
                select_suite("fleet", workers=1, **kwargs)
            # The selection never generates the summary itself
            summarize.assert_not_called()
            return ask.call_args.args[1]

        state = run(pr_summary="LLM SUMMARY")
        self.assertIn("LLM SUMMARY", state)
        self.assertNotIn("RAW DIFF", state)
        self.assertIn("RAW DIFF", run())

    def test_generate_pr_summary_makes_one_llm_call(self):
        module = "tasks.libs.dynamic_test.jev.jev_e2e_selector"
        with (
            patch(f"{module}.fetch_ddci_metadata", return_value=None),
            patch(f"{module}.fetch_pr_info", return_value={"title": "T"}),
            patch(f"{module}.changed_files", return_value=([("a.go", "")], "base")),
            patch(f"{module}.pr_diff", return_value="RAW DIFF"),
            patch(f"{module}.get_ai_gateway_token", return_value="fake"),
            patch(f"{module}.summarize_pr", return_value="LLM SUMMARY") as summarize,
            patch.dict("os.environ", {}, clear=True),
        ):
            self.assertEqual(generate_pr_summary(), "LLM SUMMARY")
        summarize.assert_called_once()
        self.assertEqual(summarize.call_args.args, ("fake", {"title": "T"}, [("a.go", "")], "base", "RAW DIFF"))
        self.assertEqual(summarize.call_args.kwargs["model"], DEFAULT_SUMMARY_MODEL)
