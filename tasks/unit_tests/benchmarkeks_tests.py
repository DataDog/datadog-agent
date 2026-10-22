import os
import tempfile
import time
import unittest
from unittest.mock import patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.testing.benchmarkeks import (
    Baseline,
    BaselineNotFoundError,
    Commit,
    find_baseline,
    format_dotenv,
    index_pipeline_ids,
    is_missing_image_error,
    parse_rev_list,
)
from tasks.new_e2e_tests import benchmarkeks_baseline

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
            "v200-a6c0b31b-amd64\n",
            "v150-73f0a2f0-amd64",
            "v150-73f0a2f0-7-amd64",
            "v150-73f0a2f-amd64",
            "latest",
        ]
        self.assertEqual(index_pipeline_ids(tags), {"a6c0b31b": [300, 200, 100], "73f0a2f0": [150]})

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
            {"a6c0b31b": [300, 200], "73f0a2f0": [100]},
            {("a6c0b31b", 200), ("73f0a2f0", 100)},
        )
        self.assertEqual(baseline, Baseline(SHA_A, 200))
        self.assertEqual(checked, [("a6c0b31b", 300), ("a6c0b31b", 200)])

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
        return {
            f"crane digest registry.ddbuild.io/ci/datadog-agent/{ref}": result
            for ref in [
                f"agent:v{pipeline_id}-{short_sha}-7-amd64",
                f"agent:v{pipeline_id}-{short_sha}-7-arm64",
                f"cluster-agent:v{pipeline_id}-{short_sha}-amd64",
                f"cluster-agent:v{pipeline_id}-{short_sha}-arm64",
            ]
        }

    def context(self, start, results):
        return MockContext(
            run={
                f"git rev-list --first-parent --timestamp --max-count=1000 {start}": Result(self.rev_list),
                "crane ls registry.ddbuild.io/ci/datadog-agent/cluster-agent": Result(
                    "v300-a6c0b31b-amd64\nv300-a6c0b31b-arm64\nv250-73f0a2f0-amd64\nv250-73f0a2f0-arm64\n"
                    "v200-73f0a2f0-amd64\nv100-d62efc99-amd64\n"
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
                **self.digest_results(250, "73f0a2f0", Result("sha256:1234")),
            },
        )
        benchmarkeks_baseline(ctx, output_file=self.output_file)
        variables = self.read_output()
        self.assertEqual(variables["E2E_BENCHMARKEKS_BASELINE_SHA"], SHA_B)
        self.assertEqual(
            variables["E2E_BENCHMARKEKS_BASELINE_AGENT_IMAGE"],
            "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa:250-73f0a2f0-linux",
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
    def test_registry_error(self):
        ctx = self.context(
            "'HEAD~1'",
            self.digest_results(
                300, "a6c0b31b", Result(exited=1, stderr='Error: Get "https://registry.ddbuild.io/v2/": Forbidden')
            ),
        )
        with self.assertRaisesRegex(Exit, "Could not check whether .* exists"):
            benchmarkeks_baseline(ctx, output_file=self.output_file)
