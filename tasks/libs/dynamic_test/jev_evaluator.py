"""Jev-flavored DatadogDynTestEvaluator.

Subclass of the dynamic test evaluator (tasks/libs/dynamic_test/evaluator.py)
with two adaptations for evaluating the Jev-based selection on e2e jobs:

- list_tests_for_job drops the `env:prod` filter of the base class: the e2e
  jobs tag their test events with `env:nativetest` (see the .new_e2e_template
  in .gitlab/test/e2e/e2e_templates.yml), which the base query would exclude,
  making every e2e job evaluate against an empty executed set
- used together with JevDynTestExecutor (tasks/libs/dynamic_test/jev_selection.py),
  which provides the full test universe (every entry point under
  test/new-e2e/tests) instead of the coverage index's known tests
"""

from __future__ import annotations

from tasks.libs.common.datadog_api import get_ci_test_events
from tasks.libs.dynamic_test.evaluator import DatadogDynTestEvaluator, ExecutedTest

PIPELINE_NAME = "DataDog/datadog-agent"


class JevDatadogDynTestEvaluator(DatadogDynTestEvaluator):
    """DatadogDynTestEvaluator whose executed-test query matches e2e events.

    Same query as the base class minus the env filter (e2e test events carry
    env:nativetest, not env:prod), and minus the job-name quoting issues of
    matrix job names (names embed their variables, e.g.
    `new-e2e-fleet-config: [linux, --run "TestFleetConfig$"]`).
    """

    def list_tests_for_job(self, job_name: str) -> list[ExecutedTest]:
        escaped_job_name = job_name.replace('"', '\\"')
        events = get_ci_test_events(
            f"@ci.pipeline.name:{PIPELINE_NAME} @ci.pipeline.id:{self.pipeline_id} @ci.job.name:\"{escaped_job_name}\"",
            3,
        )

        tests: list[ExecutedTest] = []
        for item in events:
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
