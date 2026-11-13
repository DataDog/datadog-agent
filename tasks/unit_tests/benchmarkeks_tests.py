import json
import os
import tempfile
import time
import unittest
from datetime import UTC, datetime
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.testing.benchmarkeks import (
    COMMENT_TITLE,
    FAILED_BUTTON,
    INCOMPLETE_BUTTON,
    LEGACY_COMMENT_TITLE,
    PASSED_BUTTON,
    REGRESSION_BUTTON,
    RUN_BUTTON,
    RUNNING_BUTTON,
    Baseline,
    BaselineNotFoundError,
    Button,
    Commit,
    done_comment,
    find_baseline,
    format_dotenv,
    index_pipeline_ids,
    is_missing_image_error,
    outcome_button,
    parse_rev_list,
    pending_comment,
    read_reports,
    running_comment,
)
from tasks.new_e2e_tests import benchmarkeks_baseline, benchmarkeks_comment

SHA_A = "a6c0b31b" + "1" * 32
SHA_B = "73f0a2f0" + "2" * 32
SHA_C = "d62efc99" + "3" * 32
DAY = 24 * 3600


class TestBaseline(unittest.TestCase):
    def test_images(self):
        baseline = Baseline(SHA_A, 144458951)
        self.assertEqual(
            baseline.dotenv_variables(),
            {
                "E2E_BENCHMARKEKS_BASELINE_SHA": SHA_A,
                "E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE": "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa:144458951-a6c0b31b-linux",
                "E2E_BENCHMARKEKS_BASELINE_CLUSTER_AGENT_IMAGE": "669783387624.dkr.ecr.us-east-1.amazonaws.com/cluster-agent-qa:144458951-a6c0b31b",
                "BENCHMARKEKS_BASELINE_AGENT_SOURCES": "registry.ddbuild.io/ci/datadog-agent/agent:v144458951-a6c0b31b-7-amd64,registry.ddbuild.io/ci/datadog-agent/agent:v144458951-a6c0b31b-7-arm64",
                "BENCHMARKEKS_BASELINE_CLUSTER_AGENT_SOURCES": "registry.ddbuild.io/ci/datadog-agent/cluster-agent:v144458951-a6c0b31b-amd64,registry.ddbuild.io/ci/datadog-agent/cluster-agent:v144458951-a6c0b31b-arm64",
                "BENCHMARKEKS_BASELINE_AGENT_DESTINATION": "agent-qa:144458951-a6c0b31b-linux",
                "BENCHMARKEKS_BASELINE_CLUSTER_AGENT_DESTINATION": "cluster-agent-qa:144458951-a6c0b31b",
            },
        )

    def test_format_dotenv(self):
        self.assertEqual(format_dotenv({"A": "1", "B": "x:y,z"}), "A=1\nB=x:y,z\n")


class TestParsing(unittest.TestCase):
    def test_parse_rev_list(self):
        self.assertEqual(
            parse_rev_list(f"1791917936 {SHA_A}\n1791916647 {SHA_B}\n\n"),
            [Commit(SHA_A, 1791917936), Commit(SHA_B, 1791916647)],
        )

    def test_index_pipeline_ids(self):
        tags = [
            "v100-a6c0b31b-amd64",
            "v100-a6c0b31b-arm64",
            "v100-a6c0b31b-fips-amd64",
            "v300-a6c0b31b-amd64",
            "v200-a6c0b31b-amd64",
            "v150-73f0a2f0-amd64",
            "v150-73f0a2f0-7-amd64",
            "v150-73f0a2f-amd64",
            "latest",
        ]
        self.assertEqual(index_pipeline_ids(tags), {"a6c0b31b": [100, 200, 300], "73f0a2f0": [150]})

    def test_is_missing_image_error(self):
        self.assertTrue(
            is_missing_image_error(
                "Error: GET https://registry.ddbuild.io/v2/ci/datadog-agent/agent/manifests/v1-deadbeef-7-amd64: "
                "MANIFEST_UNKNOWN: manifest unknown; unknown tag=v1-deadbeef-7-amd64"
            )
        )
        self.assertFalse(
            is_missing_image_error('Error: Get "https://registry-auth.ddbuild.io/token?scope=repository": Forbidden')
        )


class TestFindBaseline(unittest.TestCase):
    now = 2_000_000_000
    oldest = now - 4 * DAY

    def find(self, candidates, pipeline_ids, complete):
        checked = []

        def images_exist(baseline):
            checked.append((baseline.short_sha, baseline.pipeline_id))
            return (baseline.short_sha, baseline.pipeline_id) in complete

        return find_baseline(candidates, pipeline_ids, images_exist, self.oldest), checked

    def test_first_complete_pipeline_of_first_candidate(self):
        baseline, checked = self.find(
            [Commit(SHA_A, self.now), Commit(SHA_B, self.now - DAY)],
            {"a6c0b31b": [200, 300], "73f0a2f0": [100]},
            {("a6c0b31b", 300), ("73f0a2f0", 100)},
        )
        self.assertEqual(baseline, Baseline(SHA_A, 300))
        self.assertEqual(checked, [("a6c0b31b", 200), ("a6c0b31b", 300)])

    def test_old_first_candidate(self):
        baseline, _ = self.find([Commit(SHA_A, self.now - 30 * DAY)], {"a6c0b31b": [300]}, {("a6c0b31b", 300)})
        self.assertEqual(baseline, Baseline(SHA_A, 300))

    def test_walks_first_parents(self):
        baseline, checked = self.find(
            [Commit(SHA_A, self.now), Commit(SHA_B, self.now - DAY), Commit(SHA_C, self.now - 2 * DAY)],
            {"a6c0b31b": [300], "d62efc99": [100]},
            {("d62efc99", 100)},
        )
        self.assertEqual(baseline, Baseline(SHA_C, 100))
        self.assertEqual(checked, [("a6c0b31b", 300), ("d62efc99", 100)])

    def test_too_old(self):
        with self.assertRaises(BaselineNotFoundError) as cm:
            self.find(
                [Commit(SHA_A, self.now), Commit(SHA_B, self.oldest), Commit(SHA_C, self.oldest - DAY)],
                {"73f0a2f0": [100], "d62efc99": [50]},
                {("73f0a2f0", 100), ("d62efc99", 50)},
            )
        self.assertEqual(cm.exception.commit, Commit(SHA_B, self.oldest))

    def test_no_candidate_left(self):
        with self.assertRaises(BaselineNotFoundError) as cm:
            self.find([Commit(SHA_A, self.now), Commit(SHA_B, self.now - DAY)], {}, set())
        self.assertIsNone(cm.exception.commit)


@patch("tasks.libs.releasing.json._get_release_json_value", new=lambda key: {"base_branch": "main"}[key])
# The commands are quoted the POSIX way, e.g. 'HEAD~1'
@patch("tasks.libs.common.utils.is_windows", new=lambda: False)
class TestBenchmarkeksBaselineTask(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        self.output_file = os.path.join(self.tmpdir.name, "baseline.env")
        now = int(time.time())
        self.rev_list = f"{now - DAY} {SHA_A}\n{now - 2 * DAY} {SHA_B}\n{now - 10 * DAY} {SHA_C}\n"
        self.missing = Result(
            exited=1, stderr="Error: GET https://registry.ddbuild.io/v2/...: MANIFEST_UNKNOWN: manifest unknown"
        )

    def digest_results(self, pipeline_id, short_sha, result):
        # Only the Agent images are checked with `crane digest`, the Cluster Agent ones being found in `crane ls`
        return {
            f"crane digest registry.ddbuild.io/ci/datadog-agent/agent:v{pipeline_id}-{short_sha}-7-{arch}": result
            for arch in ("amd64", "arm64")
        }

    def context(self, start, results):
        return MockContext(
            run={
                f"git rev-list --first-parent --timestamp --max-count=1000 {start}": Result(self.rev_list),
                "crane ls registry.ddbuild.io/ci/datadog-agent/cluster-agent": Result(
                    "v300-a6c0b31b-amd64\nv300-a6c0b31b-arm64\nv250-73f0a2f0-amd64\nv250-73f0a2f0-arm64\n"
                    "v200-73f0a2f0-amd64\nv200-73f0a2f0-arm64\nv100-d62efc99-amd64\n"
                ),
                **results,
            }
        )

    def read_output(self):
        with open(self.output_file) as f:
            return dict(line.split("=", 1) for line in f.read().splitlines())

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "lenaic/feature"})
    def test_dev_branch(self):
        ctx = self.context(
            SHA_A,
            {
                "git fetch origin +refs/heads/main:refs/remotes/origin/main": Result(),
                "git merge-base HEAD origin/main": Result(SHA_A),
                **self.digest_results(300, "a6c0b31b", self.missing),
                **self.digest_results(200, "73f0a2f0", Result("sha256:1234")),
                **self.digest_results(250, "73f0a2f0", Result("sha256:5678")),
            },
        )
        benchmarkeks_baseline(ctx, output_file=self.output_file)
        variables = self.read_output()
        self.assertEqual(variables["E2E_BENCHMARKEKS_BASELINE_SHA"], SHA_B)
        # The oldest pipeline of the commit, rather than a later coverage one
        self.assertEqual(
            variables["E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE"],
            "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa:200-73f0a2f0-linux",
        )

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "main"})
    def test_base_branch(self):
        ctx = self.context("'HEAD~1'", self.digest_results(300, "a6c0b31b", Result("sha256:1234")))
        benchmarkeks_baseline(ctx, output_file=self.output_file)
        self.assertEqual(self.read_output()["E2E_BENCHMARKEKS_BASELINE_SHA"], SHA_A)

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "main"})
    def test_too_old(self):
        ctx = self.context(
            "'HEAD~1'",
            {
                **self.digest_results(300, "a6c0b31b", self.missing),
                **self.digest_results(250, "73f0a2f0", self.missing),
                **self.digest_results(200, "73f0a2f0", self.missing),
            },
        )
        with self.assertRaisesRegex(Exit, f"{SHA_C}.*too old.*rebasing it onto an up-to-date main branch"):
            benchmarkeks_baseline(ctx, output_file=self.output_file)
        self.assertFalse(os.path.exists(self.output_file))

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "main"})
    def test_no_commit(self):
        self.rev_list = ""
        with self.assertRaisesRegex(Exit, "No commit found from the parent commit"):
            benchmarkeks_baseline(self.context("'HEAD~1'", {}), output_file=self.output_file)
        self.assertFalse(os.path.exists(self.output_file))

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "main"})
    def test_registry_error(self):
        ctx = self.context(
            "'HEAD~1'",
            self.digest_results(
                300, "a6c0b31b", Result(exited=1, stderr='Error: Get "https://registry.ddbuild.io/v2/": Forbidden')
            ),
        )
        with self.assertRaisesRegex(Exit, "Could not check whether .* exists"):
            benchmarkeks_baseline(ctx, output_file=self.output_file)

    @patch.dict(os.environ, {"CI_COMMIT_BRANCH": "main"})
    def test_tag_listing_error(self):
        ctx = self.context(
            "'HEAD~1'",
            {
                "crane ls registry.ddbuild.io/ci/datadog-agent/cluster-agent": Result(
                    exited=1, stderr='Error: Get "https://registry.ddbuild.io/v2/": Forbidden'
                )
            },
        )
        with self.assertRaisesRegex(Exit, "(?s)Could not list the tags of .*Forbidden"):
            benchmarkeks_baseline(ctx, output_file=self.output_file)
        self.assertFalse(os.path.exists(self.output_file))


JOB_URL = "https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/2222762855"
PIPELINE_URL = "https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines/150947315"


def job(name, web_url=None):
    return SimpleNamespace(name=name, web_url=web_url)


class TestButton(unittest.TestCase):
    def test_badge_url(self):
        # The current button, whose URL the comments of past runs link to
        self.assertEqual(
            RUN_BUTTON.badge_url(),
            "https://img.shields.io/badge/%E2%96%B6%20Run%20the%20benchmark-632CA6?style=for-the-badge",
        )

    def test_escapes(self):
        # A single dash would separate the text from the color
        self.assertEqual(
            Button("a-b_c d", "FFFFFF").badge_url(),
            "https://img.shields.io/badge/a--b__c%20d-FFFFFF?style=for-the-badge",
        )

    def test_markdown(self):
        self.assertEqual(
            RUNNING_BUTTON.markdown(JOB_URL),
            f"[![\N{HOURGLASS WITH FLOWING SAND} Benchmark running]({RUNNING_BUTTON.badge_url()})]({JOB_URL})",
        )


class TestPendingComment(unittest.TestCase):
    def test_button(self):
        comment = pending_comment(JOB_URL, PIPELINE_URL)
        self.assertIn(f"\n\n{RUN_BUTTON.markdown(JOB_URL)}\n", comment)
        self.assertIn("then click **Run job**", comment)
        self.assertNotIn(PIPELINE_URL, comment)

    def test_pipeline_link(self):
        comment = pending_comment(None, PIPELINE_URL)
        self.assertIn(f"`new-e2e-containers-benchmarkeks-baseline` of [the pipeline]({PIPELINE_URL})", comment)
        self.assertNotIn(RUN_BUTTON.badge_url(), comment)


class TestRunningComment(unittest.TestCase):
    def test_button(self):
        comment = running_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", datetime(2026, 11, 13, 14, 3, tzinfo=UTC))
        self.assertTrue(comment.startswith(RUNNING_BUTTON.markdown(JOB_URL) + "\n\n"))
        self.assertIn(f"commit `a6c0b31b` started at 2026-11-13 14:03 UTC in [its pipeline]({PIPELINE_URL})", comment)


def report(*verdicts):
    return {"results": [{"verdict": verdict} for verdict in verdicts]}


def write_file(directory, name, content):
    with open(os.path.join(directory, name), "w", encoding="utf-8") as f:
        f.write(content)


class TestOutcomeButton(unittest.TestCase):
    def test_outcomes(self):
        for name, report_, button in [
            ("null results", {"results": None}, PASSED_BUTTON),
            ("unchanged", report("unchanged", "unchanged"), PASSED_BUTTON),
            ("improvement", report("unchanged", "improvement"), PASSED_BUTTON),
            ("error", report("improvement", "error"), INCOMPLETE_BUTTON),
            ("regression", report("improvement", "error", "regression"), REGRESSION_BUTTON),
        ]:
            with self.subTest(name):
                self.assertEqual(outcome_button(report_), button)


class TestDoneComment(unittest.TestCase):
    def test_report(self):
        comment = done_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", report("regression"), "Report\n")
        self.assertEqual(
            comment,
            f"{REGRESSION_BUTTON.markdown(JOB_URL)}\n\n"
            f"Results of the benchmark of commit `a6c0b31b`, in [its pipeline]({PIPELINE_URL}).\n\nReport\n",
        )

    def test_failed(self):
        comment = done_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", None, None)
        self.assertTrue(comment.startswith(FAILED_BUTTON.markdown(JOB_URL) + "\n\n"))
        self.assertIn(f"failed before comparing the versions: see the log of [its job]({JOB_URL})", comment)

    def test_markdown_missing(self):
        # The JSON report alone doesn't make the benchmark pass
        comment = done_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", report("unchanged"), None)
        self.assertTrue(comment.startswith(FAILED_BUTTON.markdown(JOB_URL) + "\n\n"))


class TestReadReports(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)

    def test_reports(self):
        write_file(self.tmpdir.name, "report.json", json.dumps(report("unchanged")))
        write_file(self.tmpdir.name, "report.md", "\N{MEDIUM WHITE CIRCLE} unchanged\n")
        self.assertEqual(read_reports(self.tmpdir.name), (report("unchanged"), "\N{MEDIUM WHITE CIRCLE} unchanged\n"))

    def test_missing(self):
        self.assertEqual(read_reports(self.tmpdir.name), (None, None))
        self.assertEqual(read_reports(""), (None, None))

    def test_truncated(self):
        # The benchmark was killed while writing them
        write_file(self.tmpdir.name, "report.json", '{"results": [')
        with open(os.path.join(self.tmpdir.name, "report.md"), "wb") as f:
            f.write("\N{MEDIUM WHITE CIRCLE}".encode()[:2])
        self.assertEqual(read_reports(self.tmpdir.name), (None, None))


@patch.dict(
    os.environ,
    {
        "CI_PIPELINE_ID": "150947315",
        "CI_PIPELINE_URL": PIPELINE_URL,
        "CI_COMMIT_SHA": SHA_A,
        "CI_COMMIT_BRANCH": "lenaic/benchmark",
    },
)
class TestBenchmarkeksCommentTask(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)

    def comment(self, repo, state, prs, report_dir="", dry_run=False):
        """Runs the task in state, with the pipeline jobs of repo and the pull requests prs of the branch, and returns
        the mocks of GithubAPI and pr_commenter."""
        with (
            patch("tasks.new_e2e_tests.get_gitlab_repo", return_value=repo),
            patch("tasks.libs.ciproviders.github_api.GithubAPI") as github,
            patch("tasks.github_tasks.pr_commenter") as pr_commenter,
        ):
            github.return_value.get_pr_for_branch.return_value = iter(prs)
            benchmarkeks_comment(MockContext(), state=state, report_dir=report_dir, dry_run=dry_run)
        return github, pr_commenter

    def run_task(self, repo, state, report_dir=""):
        """Returns the body the task posts in state, with the pipeline jobs of repo, on the pull request of the
        commit."""
        pr = SimpleNamespace(head=SimpleNamespace(sha=SHA_A))
        github, pr_commenter = self.comment(repo, state, [pr], report_dir)
        self.assertEqual(github.return_value.get_pr_for_branch.call_args.args, ("lenaic/benchmark",))
        post, delete_legacy = pr_commenter.call_args_list
        self.assertIs(post.kwargs["pr"], pr)
        self.assertEqual(post.kwargs["title"], COMMENT_TITLE)
        self.assertEqual(
            delete_legacy.kwargs, {"title": LEGACY_COMMENT_TITLE, "pr": pr, "force_delete": True, "verbose": False}
        )
        return post.kwargs["body"]

    def repo(self, *jobs):
        repo = MagicMock()
        repo.pipelines.get.return_value.jobs.list.return_value = list(jobs)
        return repo

    def test_pending(self):
        repo = self.repo(job("new-e2e-containers-benchmarkeks-baseline", JOB_URL))
        self.assertEqual(self.run_task(repo, "pending"), pending_comment(JOB_URL, PIPELINE_URL))
        self.assertEqual(repo.pipelines.get.call_args.args, ("150947315",))
        # Whatever its status: the job waits for the comment before being manual
        self.assertNotIn("scope", repo.pipelines.get.return_value.jobs.list.call_args.kwargs)

    def test_pending_job_not_found(self):
        repo = self.repo(job("deploy_staging"))
        self.assertEqual(self.run_task(repo, "pending"), pending_comment(None, PIPELINE_URL))

    def test_pending_api_error(self):
        repo = self.repo()
        repo.pipelines.get.return_value.jobs.list.side_effect = RuntimeError("401 Unauthorized")
        self.assertEqual(self.run_task(repo, "pending"), pending_comment(None, PIPELINE_URL))

    def test_running(self):
        repo = self.repo(job("new-e2e-containers-benchmarkeks-init"), job("new-e2e-containers-benchmarkeks", JOB_URL))
        body = self.run_task(repo, "running")
        self.assertTrue(body.startswith(RUNNING_BUTTON.markdown(JOB_URL)))
        self.assertIn("commit `a6c0b31b`", body)

    def test_running_job_not_found(self):
        body = self.run_task(self.repo(job("deploy_staging")), "running")
        self.assertTrue(body.startswith(RUNNING_BUTTON.markdown(PIPELINE_URL)))

    def test_done(self):
        write_file(self.tmpdir.name, "report.json", json.dumps(report("unchanged")))
        write_file(self.tmpdir.name, "report.md", "Report\n")
        repo = self.repo(job("new-e2e-containers-benchmarkeks", JOB_URL))
        self.assertEqual(
            self.run_task(repo, "done", report_dir=self.tmpdir.name),
            done_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", report("unchanged"), "Report\n"),
        )

    def test_done_without_report(self):
        repo = self.repo(job("new-e2e-containers-benchmarkeks", JOB_URL))
        self.assertEqual(
            self.run_task(repo, "done", report_dir=self.tmpdir.name),
            done_comment(JOB_URL, PIPELINE_URL, "a6c0b31b", None, None),
        )

    def test_superseded_pipeline(self):
        # A new commit was pushed: its pipeline owns the comment
        repo = self.repo()
        with patch("builtins.print") as print_:
            _, pr_commenter = self.comment(repo, "done", [SimpleNamespace(head=SimpleNamespace(sha=SHA_B))])
        pr_commenter.assert_not_called()
        # Nor are the jobs of the pipeline looked up
        repo.pipelines.get.assert_not_called()
        # The first call: the invoke logger may print after the task
        self.assertIn("The head of the pull request is now 73f0a2f0", print_.call_args_list[0].args[0])

    def test_no_pull_request(self):
        repo = self.repo()
        with patch("builtins.print"):
            _, pr_commenter = self.comment(repo, "running", [])
        pr_commenter.assert_not_called()
        repo.pipelines.get.assert_not_called()

    def test_invalid_state(self):
        with self.assertRaisesRegex(Exit, "Invalid state started"):
            benchmarkeks_comment(MockContext(), state="started")

    def test_dry_run(self):
        repo = self.repo(job("new-e2e-containers-benchmarkeks-baseline", JOB_URL))
        with patch("builtins.print") as print_:
            github, pr_commenter = self.comment(repo, "pending", [], dry_run=True)
        # No pull request needed
        github.assert_not_called()
        pr_commenter.assert_not_called()
        print_.assert_any_call(pending_comment(JOB_URL, PIPELINE_URL))
