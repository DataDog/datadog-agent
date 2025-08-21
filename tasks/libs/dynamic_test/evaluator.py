"""
Evaluator for Dynamic Test Index effectiveness.

- Base class loads context, backend and executor.
- evaluate() computes, for each job in the index:
  - actual executed tests (implementation-specific via _query_executed_tests_per_job)
  - predicted tests from the index via the provided executor
- Subclasses should implement data retrieval.

Environment variables expected by concrete implementations apply as usual (e.g., for Datadog API client: DD_SITE, DD_API_KEY, DD_APP_KEY).
"""

from __future__ import annotations

import datetime
from abc import ABC, abstractmethod
from dataclasses import dataclass

from invoke import Context

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.datadog_api import create_count, get_ci_test_events, send_event, send_metrics
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind


@dataclass
class EvaluationResult:
    """Results of evaluating a dynamic test index against actual test execution.

    Compares what tests were actually executed in CI against what the dynamic
    test index predicted should be executed, providing metrics for index effectiveness.

    Attributes:
        job_name: Name of the CI job being evaluated
        actual_executed_tests: Set of test names that were actually executed
        predicted_executed_tests: Set of test names the index predicted should run
        not_executed_failing_tests: Set of failing tests that were NOT predicted
                                   (critical misses that could hide failures)
    """

    job_name: str
    actual_executed_tests: set[str]
    predicted_executed_tests: set[str]
    not_executed_failing_tests: set[str]

    def actual_count(self) -> int:
        return len(self.actual_executed_tests)

    def predicted_count(self) -> int:
        return len(self.predicted_executed_tests)

    def not_executed_failing_count(self) -> int:
        return len(self.not_executed_failing_tests)

    def pretty_print(self):
        header = color_message(f"[Job] {self.job_name}", Color.MAGENTA)
        print(header)

        actual = sorted(self.actual_executed_tests)
        predicted = sorted(self.predicted_executed_tests)
        removed = sorted(self.actual_executed_tests - self.predicted_executed_tests)
        added = sorted(self.predicted_executed_tests - self.actual_executed_tests)
        common = sorted(self.actual_executed_tests & self.predicted_executed_tests)

        # Git-diff style headers
        print(color_message("--- actual", Color.GREY))
        print(color_message("+++ predicted", Color.GREY))

        # Context (common)
        for t in common:
            print("  " + t)

        # Removed from predicted (were executed but not predicted)
        if removed:
            for t in removed:
                suffix = ""
                if t in self.not_executed_failing_tests:
                    suffix = color_message(" [FAILED]", Color.RED)
                print(color_message(f"- {t}", Color.RED) + suffix)

        # Added by prediction (predicted but not executed)
        if added:
            for t in added:
                print(color_message(f"+ {t}", Color.GREEN))

        # Print a big warning for failing tests that were not executed
        if self.not_executed_failing_tests:
            print(color_message("WARNING: The following failing tests would not have been executed:", Color.RED))
            for t in self.not_executed_failing_tests:
                print(color_message(f"- {t}", Color.RED))
        print()

        # Quick summary
        print(
            color_message(
                f"summary: actual={len(actual)} predicted={len(predicted)} → +{len(added)} / -{len(removed)}",
                Color.BLUE,
            )
        )


@dataclass
class ExecutedTest:
    """Represents a test that was executed in a CI pipeline.

    Contains metadata about test execution including status and reliability
    information needed for evaluation.

    Attributes:
        name: The test identifier/name
        status: Test result status (e.g., "passed", "failed")
        pipeline_id: CI pipeline identifier
        job_id: CI job identifier
        job_name: CI job name
        unreliable_status: True if the test status is unreliable (e.g., flaky test)
                          When True, failed tests are not counted as critical misses
    """

    name: str
    status: str
    pipeline_id: str
    job_id: str
    job_name: str
    unreliable_status: bool


class DynTestEvaluator(ABC):
    """Abstract base class for evaluating dynamic test index effectiveness.

    Evaluates how well a dynamic test index predicts which tests should be executed
    by comparing index predictions against actual test execution in CI pipelines.

    The evaluator:
    1. Loads context, backend, and executor for the evaluation
    2. Retrieves actual executed tests for each job (implementation-specific)
    3. Compares with predicted tests from the index
    4. Identifies critical misses (failing tests that weren't predicted)
    5. Generates evaluation results and metrics

    Implementations should provide concrete methods for retrieving test execution
    data from their specific CI/monitoring systems (e.g., Datadog API, Jenkins API).

    Environment Variables:
        Concrete implementations may require specific environment variables
        (e.g., DD_SITE, DD_API_KEY, DD_APP_KEY for Datadog implementation).
    """

    def __init__(self, ctx: Context, kind: IndexKind, executor: DynTestExecutor, pipeline_id: str):
        """Initialize the evaluator.

        Note: The index is not loaded during initialization. Call initialize()
        to load the index and handle any errors with Datadog reporting.

        Args:
            ctx: Invoke context for running shell commands
            kind: Type of index being evaluated
            executor: Executor with lazy index loading capability
            pipeline_id: CI pipeline ID to evaluate against
        """
        self.ctx = ctx
        self.executor = executor
        self.kind = kind
        self.pipeline_id = pipeline_id

    @abstractmethod
    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        """Retrieve tests that were executed for a specific job in the target pipeline.

        This method must be implemented by concrete subclasses to query their
        specific CI/monitoring system for test execution data.

        Args:
            job_name: Name of the CI job to get test execution data for

        Returns:
            list[ExecutedTest]: List of tests that were executed in the specified job.
                               Each ExecutedTest should include name, status, and metadata.

        Note:
            - Should only return tests from the pipeline specified in __init__
            - Test names should match the format used in the dynamic test index
            - Status should indicate pass/fail state for miss detection
            - unreliable_status flag should be set for flaky/unreliable tests
        """
        raise NotImplementedError

    def initialize(self):
        """Initialize the evaluator's index and send error events on failure.

        Triggers the executor's lazy index loading and handles any errors
        by sending appropriate events to Datadog for monitoring.

        Returns:
            bool: True if initialization succeeded, False if it failed
        """
        try:
            self.executor.init_index()
            self.index = self.executor.index()
            return True
        except RuntimeError as e:
            error_message = str(e)
            if "No ancestor commit found" in error_message:
                self._send_error_to_datadog(
                    error_type="index_not_found",
                    error_message=f"No ancestor commit with index found for {self.executor.commit_sha}. Available indexed commits may be too old or missing.",
                )
            else:
                self._send_error_to_datadog(
                    error_type="index_initialization_failed",
                    error_message=error_message,
                )
            return False
        except Exception as e:
            self._send_error_to_datadog(
                error_type="unexpected_error",
                error_message=f"Unexpected error initializing index: {str(e)}",
            )
            return False

    def _send_error_to_datadog(self, error_type: str, error_message: str):
        """Send error event to Datadog when evaluation fails.

        Reports issues like missing indexes, backend failures, or other evaluation problems
        as Datadog events to enable monitoring and alerting on dynamic test system health.

        Args:
            error_type: Type of error (e.g., "index_not_found", "backend_error", "evaluation_failed")
            error_message: Detailed error message for debugging
            commit_sha: Commit SHA that caused the error (if known)
        """
        event_title = f"Dynamic Test Evaluator Error: {error_type}"
        event_text = f"""
Dynamic test evaluation failed for pipeline {self.pipeline_id}

**Error Type**: {error_type}
**Index Kind**: {getattr(self, 'kind', 'unknown')}
**Commit SHA**: {self.executor.commit_sha}
**Pipeline ID**: {self.pipeline_id}

**Error Details**:
{error_message}

This indicates an issue with the dynamic test system that may affect CI performance.
        """.strip()

        try:
            send_event(
                title=event_title,
                text=event_text,
                tags=[
                    f"pipeline_id:{self.pipeline_id}",
                    f"index_kind:{getattr(self, 'kind', 'unknown')}",
                    f"error_type:{error_type}",
                    f"commit_sha:{self.executor.commit_sha}",
                    "service:dynamic_test_evaluator",
                ],
            )
        except Exception as e:
            # Fallback to console logging if Datadog event sending fails
            print(f"Failed to send error event to Datadog: {e}")
            print(f"Original error - {event_title}: {error_message}")

    def evaluate(self, changes: list[str]) -> list[EvaluationResult]:
        jobs = list(self.index.to_dict().keys())

        predicted_tests_per_job = self.executor.tests_to_run_per_job(changes)
        results: list[EvaluationResult] = []
        for job in jobs:
            evaluation_result = self._evaluate_job(
                job, self.list_tests_for_job(job), predicted_tests_per_job.get(job, set())
            )
            results.append(evaluation_result)
        return results

    def print_summary(self, results: list[EvaluationResult]):
        global_actual_count = 0
        global_predicted_count = 0
        global_not_executed_failing_count = 0

        for result in results:
            print("=" * 80)
            print(f"Index kind: {self.kind.value}")
            result.pretty_print()
            global_actual_count += result.actual_count()
            global_predicted_count += result.predicted_count()
            global_not_executed_failing_count += result.not_executed_failing_count()

        print("=" * 80)
        print(
            color_message(
                f"Global summary: actual={global_actual_count} predicted={global_predicted_count}", Color.BLUE
            )
        )
        if global_not_executed_failing_count > 0:
            print(
                color_message(
                    f"WARNING: {global_not_executed_failing_count} failing tests would not have been executed",
                    Color.RED,
                )
            )

    def send_stats_to_datadog(self, results: list[EvaluationResult]):
        series = []
        for result in results:
            series.append(
                create_count(
                    metric_name="datadog_agent.ci.dynamic_test.evaluator.actual_executed_tests",
                    timestamp=int(datetime.datetime.now().timestamp()),
                    value=result.actual_count(),
                    tags=["pipeline_id:" + self.pipeline_id, "index_kind:" + self.kind.value, "job:" + result.job_name],
                )
            )
            series.append(
                create_count(
                    metric_name="datadog_agent.ci.dynamic_test.evaluator.predicted_executed_tests",
                    timestamp=int(datetime.datetime.now().timestamp()),
                    value=result.predicted_count(),
                    tags=["pipeline_id:" + self.pipeline_id, "index_kind:" + self.kind.value, "job:" + result.job_name],
                )
            )
            series.append(
                create_count(
                    metric_name="datadog_agent.ci.dynamic_test.evaluator.not_executed_failing_tests",
                    timestamp=int(datetime.datetime.now().timestamp()),
                    value=result.not_executed_failing_count(),
                    tags=["pipeline_id:" + self.pipeline_id, "index_kind:" + self.kind.value, "job:" + result.job_name],
                )
            )
        send_metrics(series)

    def _evaluate_job(
        self, job: str, current_job_tests: list[ExecutedTest], predicted_tests: set[str]
    ) -> EvaluationResult:
        # Only consider indexed tests, other tests are test the system is currently not able to determine whether they should be executed or not.
        indexed_tests = self.index.get_indexed_tests_for_job(job)
        actual_executed_tests = {test.name for test in current_job_tests if test.name in indexed_tests}
        predicted_executed_tests = predicted_tests & indexed_tests
        not_executed_failing_tests = set()
        for test in current_job_tests:
            if test.name not in indexed_tests:
                continue
            if test.status == "failed" and not test.unreliable_status and test.name not in predicted_executed_tests:
                not_executed_failing_tests.add(test.name)

        return EvaluationResult(job, actual_executed_tests, predicted_executed_tests, not_executed_failing_tests)


class DatadogDynTestEvaluator(DynTestEvaluator):
    """Datadog API-based implementation of DynTestEvaluator.

    Retrieves test execution data from Datadog's CI Visibility API to evaluate
    dynamic test index effectiveness.

    Required Environment Variables:
        DD_SITE: Datadog site (e.g., datadoghq.com)
        DD_API_KEY: Datadog API key for authentication
        DD_APP_KEY: Datadog application key for API access

    The implementation:
    - Queries Datadog CI test events API for the specified pipeline
    - Filters for root-level tests (not sub-tests)
    - Extracts test name, status, and flaky test indicators
    - Returns ExecutedTest objects for evaluation
    """

    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        """Retrieve tests executed in a specific job using Datadog CI API.

        Queries Datadog's CI test events API for tests executed in the specified
        job within this evaluator's target pipeline.

        Args:
            job_name: Name of the CI job to query test execution for

        Returns:
            list[ExecutedTest]: Tests executed in the job, with status and metadata

        Note:
            - Only returns root-level tests (filters out sub-tests with '/' in name)
            - Sets unreliable_status=True for tests marked as flaky by Datadog
            - Queries up to 3 days of historical data
        """
        response = get_ci_test_events(
            f'@ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:{self.pipeline_id} @ci.job.name:"{job_name.replace('"', '\\"')}"',
            3,
        )

        tests: list[ExecutedTest] = []
        for item in response.get("data", []):
            attrs = item.get("attributes", {})
            attrs = attrs.get("attributes", {})
            test_attrs = attrs.get("test", {})
            ci_attrs = attrs.get("ci", {})
            job_attrs = ci_attrs.get("job", {})
            pipeline_attrs = ci_attrs.get("pipeline", {})
            # Only consider root tests, not sub-tests
            if not test_attrs.get("name") or len(test_attrs.get("name").split("/")) > 1:
                continue
            tests.append(
                ExecutedTest(
                    name=test_attrs.get("name"),
                    status=test_attrs.get("status"),
                    pipeline_id=pipeline_attrs.get("id"),
                    job_id=job_attrs.get("id"),
                    job_name=job_attrs.get("name"),
                    unreliable_status=test_attrs.get("agent_is_flaky_failure", "false") == "true",
                )
            )
        return tests
