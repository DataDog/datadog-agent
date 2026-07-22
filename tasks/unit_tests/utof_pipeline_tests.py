"""Tests for pipeline-wide UTOF aggregation and report formatting."""

import json
import unittest
from unittest.mock import MagicMock

from tasks.libs.testing.utof.models import (
    UTOFAttempt,
    UTOFDocument,
    UTOFFailure,
    UTOFFlaky,
    UTOFMetadata,
    UTOFSummary,
    UTOFTestResult,
)
from tasks.libs.testing.utof.pipeline import JobUTOFResult, aggregate_results, fetch_pipeline_utof_results
from tasks.libs.testing.utof.pipeline_report import format_pipeline_report


def _doc(tests, total, passed=0, failed=0, flaky=0, status="pass") -> UTOFDocument:
    return UTOFDocument(
        metadata=UTOFMetadata(test_system="unit"),
        summary=UTOFSummary(total=total, passed=passed, failed=failed, flaky=flaky, status=status),
        tests=tests,
    )


def _job(name, status="success", utof=None, error=None) -> JobUTOFResult:
    return JobUTOFResult(
        job_name=name, job_url=f"https://gitlab.example/jobs/{name}", job_status=status, utof=utof, error=error
    )


def _mock_gitlab_job(job_id, name, stage, status):
    job = MagicMock()
    job.id = job_id
    job.name = name
    job.stage = stage
    job.status = status
    job.web_url = f"https://gitlab.example/jobs/{job_id}"
    return job


def _mock_gitlab_repo(artifacts_by_job_id):
    """artifacts_by_job_id: {job_id: (artifact_name, bytes)}."""

    def fake_jobs_get(job_id, **_kwargs):
        proj_job = MagicMock()
        if job_id in artifacts_by_job_id:
            name, data = artifacts_by_job_id[job_id]

            def artifact(fname, _name=name, _data=data):
                if fname == _name:
                    return _data
                raise Exception("404 Not Found")

            proj_job.artifact.side_effect = artifact
        else:
            proj_job.artifact.side_effect = Exception("404 Not Found")
        return proj_job

    repo = MagicMock()
    repo.jobs.get.side_effect = fake_jobs_get
    return repo


class TestFetchPipelineUtofResults(unittest.TestCase):
    """Test job filtering/fetching against a mocked GitLab API (fetch_pipeline_utof_results)."""

    def test_only_relevant_stages_and_failed_status_probed_and_returned(self):
        fail_doc = UTOFDocument.from_dict(
            {
                "version": "1.0.0",
                "summary": {"total": 1, "failed": 1, "status": "fail"},
                "tests": [{"id": "x", "name": "TestX", "full_name": "TestX", "package": "pkg", "status": "fail"}],
            }
        )
        jobs = [
            _mock_gitlab_job(1, "unit_tests-linux-x64", "source_test", "failed"),
            _mock_gitlab_job(2, "deploy_something", "deploy", "failed"),  # irrelevant stage
            _mock_gitlab_job(3, "unit_tests-macos", "source_test", "success"),  # not failed, never probed
        ]
        pipeline = MagicMock()
        pipeline.jobs.list.return_value = jobs
        repo = _mock_gitlab_repo({1: ("test_output_unified.json", json.dumps(fail_doc.to_dict()).encode())})

        results = fetch_pipeline_utof_results(repo, pipeline)

        self.assertEqual([r.job_name for r in results], ["unit_tests-linux-x64"])
        repo.jobs.get.assert_any_call(1, lazy=True)
        self.assertNotIn(2, [call.args[0] for call in repo.jobs.get.call_args_list])
        self.assertNotIn(3, [call.args[0] for call in repo.jobs.get.call_args_list])

    def test_failed_job_without_artifact_reported_as_no_data(self):
        jobs = [_mock_gitlab_job(4, "e2e_tests-aws", "e2e", "failed")]
        pipeline = MagicMock()
        pipeline.jobs.list.return_value = jobs
        repo = _mock_gitlab_repo({})

        results = fetch_pipeline_utof_results(repo, pipeline)

        self.assertEqual(len(results), 1)
        self.assertIsNone(results[0].utof)
        self.assertIsNotNone(results[0].error)
        self.assertEqual(results[0].job_status, "failed")

    def test_e2e_stage_uses_e2e_artifact_name(self):
        fail_doc = UTOFDocument.from_dict(
            {
                "version": "1.0.0",
                "summary": {"total": 1, "failed": 1, "status": "fail"},
                "tests": [
                    {"id": "x", "name": "TestX", "full_name": "TestX", "package": "test/new-e2e", "status": "fail"}
                ],
            }
        )
        jobs = [_mock_gitlab_job(5, "e2e_tests-aws", "e2e", "failed")]
        pipeline = MagicMock()
        pipeline.jobs.list.return_value = jobs
        repo = _mock_gitlab_repo({5: ("e2e_test_output_unified.json", json.dumps(fail_doc.to_dict()).encode())})

        results = fetch_pipeline_utof_results(repo, pipeline)

        self.assertEqual(len(results), 1)
        self.assertIsNotNone(results[0].utof)
        self.assertEqual(results[0].utof.tests[0].name, "TestX")

    def test_non_failed_status_jobs_skip_fetch(self):
        jobs = [
            _mock_gitlab_job(6, "unit_tests-linux-x64", "source_test", "created"),
            _mock_gitlab_job(7, "unit_tests-macos", "source_test", "running"),
            _mock_gitlab_job(8, "new-e2e-aws", "e2e", "manual"),
            _mock_gitlab_job(9, "new-e2e-gcp", "e2e", "skipped"),
            _mock_gitlab_job(10, "unit_tests-windows", "source_test", "canceled"),
            _mock_gitlab_job(11, "unit_tests-arm64", "source_test", "success"),
        ]
        pipeline = MagicMock()
        pipeline.jobs.list.return_value = jobs
        repo = _mock_gitlab_repo({})

        results = fetch_pipeline_utof_results(repo, pipeline)

        # Only failed jobs are worth probing for this report — the artifact fetch
        # itself (the expensive HTTP round trip) must never have been attempted.
        self.assertEqual(results, [])
        repo.jobs.get.assert_not_called()


class TestAggregateResults(unittest.TestCase):
    def test_failures_bucketed_from_leaf_tests(self):
        doc = _doc(
            [
                UTOFTestResult(id="a", name="TestA", full_name="TestA", package="pkg", status="pass"),
                UTOFTestResult(
                    id="b",
                    name="TestB",
                    full_name="TestB",
                    package="pkg",
                    status="fail",
                    attempts=[
                        UTOFAttempt(attempt=1, status="fail", failure=UTOFFailure(message="boom", type="assertion"))
                    ],
                ),
            ],
            total=2,
            passed=1,
            failed=1,
            status="fail",
        )
        job = _job("unit_tests-linux", utof=doc)
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])

        self.assertEqual(len(agg.failures), 1)
        failed_job, failed_test = agg.failures[0]
        self.assertIs(failed_job, job)
        self.assertEqual(failed_test.name, "TestB")
        self.assertEqual(agg.flaky, [])
        self.assertEqual(agg.no_data_jobs, [])

    def test_flaky_fail_and_flaky_pass_both_bucketed_as_flaky(self):
        doc = _doc(
            [
                UTOFTestResult(
                    id="a",
                    name="TestA",
                    full_name="TestA",
                    package="pkg",
                    status="flaky_fail",
                    flaky=UTOFFlaky(source="marker"),
                ),
                UTOFTestResult(
                    id="b",
                    name="TestB",
                    full_name="TestB",
                    package="pkg",
                    status="flaky_pass",
                    flaky=UTOFFlaky(source="washer"),
                ),
            ],
            total=2,
            passed=2,
            flaky=2,
        )
        job = _job("unit_tests-macos", utof=doc)
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])

        self.assertEqual(len(agg.flaky), 2)
        self.assertEqual({t.name for _, t in agg.flaky}, {"TestA", "TestB"})
        self.assertEqual(agg.failures, [])

    def test_nested_subtests_only_leaves_bucketed(self):
        parent = UTOFTestResult(
            id="p",
            name="TestSuite",
            full_name="TestSuite",
            package="pkg",
            status="fail",
            subtests=[
                UTOFTestResult(
                    id="s1",
                    name="SubA",
                    full_name="TestSuite/SubA",
                    package="pkg",
                    status="fail",
                    attempts=[
                        UTOFAttempt(attempt=1, status="fail", failure=UTOFFailure(message="x", type="assertion"))
                    ],
                ),
                UTOFTestResult(id="s2", name="SubB", full_name="TestSuite/SubB", package="pkg", status="pass"),
            ],
        )
        doc = _doc([parent], total=2, passed=1, failed=1, status="fail")
        job = _job("unit_tests-windows", utof=doc)
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])

        # Only the failing leaf (SubA) is bucketed, not the synthetic parent.
        self.assertEqual(len(agg.failures), 1)
        self.assertEqual(agg.failures[0][1].name, "SubA")

    def test_jobs_without_utof_go_to_no_data(self):
        job = _job("e2e_tests-aws", status="failed", error="job failed, no UTOF artifact found")
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])

        self.assertEqual(agg.no_data_jobs, [job])
        self.assertEqual(agg.failures, [])
        self.assertEqual(agg.flaky, [])

    def test_summary_totals_summed_across_jobs(self):
        doc1 = _doc(
            [UTOFTestResult(id="a", name="TestA", full_name="TestA", package="pkg", status="pass")], total=1, passed=1
        )
        doc2 = _doc(
            [
                UTOFTestResult(
                    id="b",
                    name="TestB",
                    full_name="TestB",
                    package="pkg",
                    status="fail",
                    attempts=[
                        UTOFAttempt(attempt=1, status="fail", failure=UTOFFailure(message="x", type="assertion"))
                    ],
                )
            ],
            total=1,
            failed=1,
            status="fail",
        )
        agg = aggregate_results(
            "123",
            "https://gitlab.example/pipelines/123",
            [_job("unit-linux", utof=doc1), _job("unit-macos", utof=doc2)],
        )

        self.assertEqual(agg.summary.total, 2)
        self.assertEqual(agg.summary.passed, 1)
        self.assertEqual(agg.summary.failed, 1)
        self.assertEqual(agg.summary.status, "fail")

    def test_summary_status_pass_when_no_failures(self):
        doc = _doc(
            [UTOFTestResult(id="a", name="TestA", full_name="TestA", package="pkg", status="pass")], total=1, passed=1
        )
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [_job("unit-linux", utof=doc)])
        self.assertEqual(agg.summary.status, "pass")


class TestPipelineAggregateToDict(unittest.TestCase):
    def test_to_dict_includes_expected_sections(self):
        doc = _doc(
            [
                UTOFTestResult(
                    id="b",
                    name="TestB",
                    full_name="TestB",
                    package="pkg",
                    status="fail",
                    attempts=[
                        UTOFAttempt(attempt=1, status="fail", failure=UTOFFailure(message="boom", type="assertion"))
                    ],
                )
            ],
            total=1,
            failed=1,
            status="fail",
        )
        agg = aggregate_results(
            "123",
            "https://gitlab.example/pipelines/123",
            [_job("unit-linux", utof=doc), _job("e2e-aws", status="failed", error="no artifact")],
        )
        d = agg.to_dict()

        self.assertEqual(d["pipeline_id"], "123")
        self.assertEqual(d["jobs_checked"], 2)
        self.assertEqual(len(d["failures"]), 1)
        self.assertEqual(d["failures"][0]["test"], "TestB")
        self.assertEqual(d["failures"][0]["job_name"], "unit-linux")
        self.assertEqual(d["failures"][0]["message"], "boom")
        self.assertEqual(len(d["no_data_jobs"]), 1)
        self.assertEqual(d["no_data_jobs"][0]["job_name"], "e2e-aws")

    def test_to_dict_is_json_serializable(self):
        import json

        doc = _doc(
            [UTOFTestResult(id="a", name="TestA", full_name="TestA", package="pkg", status="pass")], total=1, passed=1
        )
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [_job("unit-linux", utof=doc)])
        json.dumps(agg.to_dict())  # should not raise


class TestFormatPipelineReport(unittest.TestCase):
    def test_report_includes_job_name_and_failure(self):
        doc = _doc(
            [
                UTOFTestResult(
                    id="b",
                    name="TestB",
                    full_name="TestB",
                    package="github.com/DataDog/datadog-agent/pkg/foo",
                    status="fail",
                    attempts=[
                        UTOFAttempt(
                            attempt=1, status="fail", failure=UTOFFailure(message="expected 1, got 2", type="assertion")
                        )
                    ],
                )
            ],
            total=1,
            failed=1,
            status="fail",
        )
        job = _job("unit_tests-linux-x64", utof=doc)
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])
        report = format_pipeline_report(agg)

        self.assertIn("unit_tests-linux-x64", report)
        self.assertIn("TestB", report)
        self.assertIn("pkg/foo", report)
        self.assertIn("expected 1, got 2", report)
        self.assertIn("FAILURES FOUND", report)

    def test_report_shows_no_data_section(self):
        job = _job("e2e_tests-aws", status="failed", error="job failed, no UTOF artifact found")
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [job])
        report = format_pipeline_report(agg)

        self.assertIn("no test data", report)
        self.assertIn("e2e_tests-aws", report)
        self.assertIn("job failed, no UTOF artifact found", report)

    def test_report_all_passed_when_no_failures(self):
        doc = _doc(
            [UTOFTestResult(id="a", name="TestA", full_name="TestA", package="pkg", status="pass")], total=1, passed=1
        )
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [_job("unit-linux", utof=doc)])
        report = format_pipeline_report(agg)

        self.assertIn("ALL PASSED", report)
        self.assertNotIn("Failures", report)

    def test_report_flaky_section_present(self):
        doc = _doc(
            [
                UTOFTestResult(
                    id="a",
                    name="TestA",
                    full_name="TestA",
                    package="pkg",
                    status="flaky_fail",
                    flaky=UTOFFlaky(source="marker"),
                )
            ],
            total=1,
            passed=1,
            flaky=1,
        )
        agg = aggregate_results("123", "https://gitlab.example/pipelines/123", [_job("unit-linux", utof=doc)])
        report = format_pipeline_report(agg)

        self.assertIn("Flaky", report)
        self.assertIn("TestA", report)


if __name__ == "__main__":
    unittest.main()
