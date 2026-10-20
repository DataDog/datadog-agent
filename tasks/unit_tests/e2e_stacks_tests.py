import os
import unittest
from tempfile import TemporaryDirectory
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from invoke.context import Context

from tasks.new_e2e_tests import _stacks_cleaned_up_by_job, cleanup_job_stacks
from tasks.tools.e2e_stacks import (
    RECORDED_STACKS_FILE,
    destroy_remote_stack_api,
    init_job_consumers,
    read_recorded_stacks,
    stacks_cleanup_reason,
)


def job(name, status="created"):
    return SimpleNamespace(name=name, status=status)


class TestReadRecordedStacks(unittest.TestCase):
    def test_missing_file(self):
        with TemporaryDirectory() as tmpdir:
            self.assertEqual(read_recorded_stacks(tmpdir), [])

    def test_deduplicates(self):
        with TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, RECORDED_STACKS_FILE), "w") as f:
                f.write("stack-a\nstack-b\n\nstack-a\n")
            self.assertEqual(read_recorded_stacks(tmpdir), ["stack-a", "stack-b"])


class TestInitJobConsumers(unittest.TestCase):
    def test_consumers(self):
        jobs = [
            job("new-e2e-ssi-gke-init"),
            job("new-e2e-ssi-gke"),
            job("new-e2e-ssi-gke 1/2"),
            job("new-e2e-ssi-gke: [arm64]"),
            job("new-e2e-ssi-gke-autopilot"),
        ]
        self.assertEqual(
            [j.name for j in init_job_consumers("new-e2e-ssi-gke-init", jobs)],
            ["new-e2e-ssi-gke", "new-e2e-ssi-gke 1/2", "new-e2e-ssi-gke: [arm64]"],
        )

    def test_not_an_init_job(self):
        self.assertEqual(init_job_consumers("new-e2e-ssi-gke", [job("new-e2e-ssi-gke")]), [])


class TestStacksCleanupReason(unittest.TestCase):
    def test_canceled_job(self):
        self.assertIsNotNone(stacks_cleanup_reason("canceled", False, None))
        self.assertIsNotNone(stacks_cleanup_reason("canceled", True, [job("x", "running")]))

    def test_regular_job(self):
        self.assertIsNotNone(stacks_cleanup_reason("success", False, None))
        self.assertIsNotNone(stacks_cleanup_reason("failed", False, None))

    def test_init_job_consumers_canceled(self):
        consumers = [job("x", "canceled"), job("x 1/2", "skipped")]
        self.assertIsNotNone(stacks_cleanup_reason("success", True, consumers))
        self.assertIsNotNone(stacks_cleanup_reason("failed", True, consumers))

    def test_init_job_consumer_pending(self):
        self.assertIsNone(stacks_cleanup_reason("success", True, [job("x", "canceled"), job("x 1/2", "created")]))

    def test_init_job_without_consumer(self):
        self.assertIsNone(stacks_cleanup_reason("success", True, []))


@patch("tasks.new_e2e_tests.running_in_ci", new=MagicMock(return_value=True))
class TestCleanupJobStacks(unittest.TestCase):
    def setUp(self):
        self.tmpdir = TemporaryDirectory()
        self.addCleanup(self.tmpdir.cleanup)
        with open(os.path.join(self.tmpdir.name, RECORDED_STACKS_FILE), "w") as f:
            f.write("ci-init-1-14-e2e-eks\n")
        self.env = {
            "E2E_OUTPUT_DIR": self.tmpdir.name,
            "CI_PIPELINE_ID": "1",
            "CI_JOB_NAME": "new-e2e-containers-eks-init",
            "E2E_INIT_ONLY": "true",
        }

    def run_task(self, job_status, pipeline_jobs):
        repo = MagicMock()
        repo.pipelines.get.return_value.jobs.list.return_value = pipeline_jobs
        destroy = MagicMock(return_value=(0, "", "", ""))
        with (
            patch.dict(os.environ, {**self.env, "CI_JOB_STATUS": job_status}),
            patch("tasks.new_e2e_tests.get_gitlab_repo", return_value=repo),
            patch("tasks.new_e2e_tests.destroy_remote_stack_api", destroy),
        ):
            cleanup_job_stacks(Context())
        return destroy

    def test_init_job_with_canceled_consumer(self):
        destroy = self.run_task("success", [job("new-e2e-containers-eks", "canceled")])
        destroy.assert_called_once()
        self.assertEqual(destroy.call_args[0][0], "organization/e2eci/ci-init-1-14-e2e-eks")
        self.assertTrue(destroy.call_args.kwargs["cancel_first"])

    def test_init_job_with_pending_consumer(self):
        destroy = self.run_task("success", [job("new-e2e-containers-eks", "created")])
        destroy.assert_not_called()

    def test_canceled_job(self):
        destroy = self.run_task("canceled", [job("new-e2e-containers-eks", "created")])
        destroy.assert_called_once()

    def test_regular_job(self):
        self.env["E2E_INIT_ONLY"] = "false"
        destroy = self.run_task("success", [])
        destroy.assert_called_once()
        self.assertTrue(destroy.call_args.kwargs["cancel_first"])


@patch.dict(os.environ, {"CI_JOB_NAME": "job", "CI_JOB_ID": "2", "CI_PIPELINE_ID": "1", "CI_COMMIT_REF_NAME": "branch"})
class TestDestroyRemoteStackApi(unittest.TestCase):
    @patch("tasks.tools.e2e_stacks.run")
    def test_default(self, run):
        destroy_remote_stack_api("organization/e2eci/stack", Context())
        self.assertNotIn("cancel_first", run.call_args.kwargs["attrs"])

    @patch("tasks.tools.e2e_stacks.run")
    def test_cancel_first(self, run):
        destroy_remote_stack_api("organization/e2eci/stack", Context(), cancel_first=True)
        self.assertIn("stack_name=organization/e2eci/stack,", run.call_args.kwargs["attrs"])
        self.assertTrue(run.call_args.kwargs["attrs"].endswith(",cancel_first=bool:true"))


class TestStacksCleanedUpByJob(unittest.TestCase):
    @patch("tasks.new_e2e_tests.running_in_gitlab_ci", new=MagicMock(return_value=True))
    def test_gitlab_with_remote_cleaning(self):
        with patch.dict(os.environ, {"REMOTE_STACK_CLEANING": "true"}):
            self.assertTrue(_stacks_cleaned_up_by_job())

    @patch("tasks.new_e2e_tests.running_in_gitlab_ci", new=MagicMock(return_value=True))
    def test_gitlab_without_remote_cleaning(self):
        with patch.dict(os.environ, {"REMOTE_STACK_CLEANING": ""}):
            self.assertFalse(_stacks_cleaned_up_by_job())

    @patch("tasks.new_e2e_tests.running_in_gitlab_ci", new=MagicMock(return_value=False))
    def test_not_in_gitlab(self):
        with patch.dict(os.environ, {"REMOTE_STACK_CLEANING": "true"}):
            self.assertFalse(_stacks_cleaned_up_by_job())
