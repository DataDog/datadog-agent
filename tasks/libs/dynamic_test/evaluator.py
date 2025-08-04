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
from tasks.libs.common.datadog_api import create_count, get_ci_test_events, send_metrics
from tasks.libs.dynamic_test.executor import DynTestExecutor
from tasks.libs.dynamic_test.index import IndexKind


@dataclass
class EvaluationResult:
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
    name: str
    status: str
    pipeline_id: str
    job_id: str
    job_name: str
    unreliable_status: bool  # This field is set to true if the test status is not reliable, e.g. if the test is flaky.


class DynTestEvaluator(ABC):
    def __init__(self, ctx: Context, kind: IndexKind, executor: DynTestExecutor, pipeline_id: str):
        self.ctx = ctx
        self.executor = executor
        self.index = executor.index
        self.kind = kind
        self.pipeline_id = pipeline_id

    @abstractmethod
    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        """Return tests executed for a given job name on this evaluator's pipeline.

        Each item includes at least the test name, status, pipeline id, job id and job name.
        """
        raise NotImplementedError

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


# Implementation of DynTestEvaluator using Datadog API to fetch test results.
class DatadogDynTestEvaluator(DynTestEvaluator):
    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        """Return jobs executed for a given job name on this evaluator's pipeline.

        Each item includes at least the job id and name.
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
