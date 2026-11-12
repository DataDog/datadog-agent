import io
import json
import unittest
from contextlib import redirect_stdout
from datetime import datetime, timedelta, timezone
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from datadog_api_client import ApiClient, Configuration
from datadog_api_client.v2.api.ci_visibility_tests_api import CIVisibilityTestsApi
from datadog_api_client.v2.model.ci_app_test_events_response import CIAppTestEventsResponse

from tasks.libs.dynamic_test.evaluator import (
    DatadogDynTestEvaluator,
    DynTestEvaluator,
    ExecutedTest,
    executed_tests_from_events,
    pipeline_jobs_allowed_to_fail,
)
from tasks.libs.dynamic_test.index import DynamicTestIndex, IndexKind
from tasks.libs.dynamic_test.telemetry import ConsoleTelemetryHandler


class _FakeEvaluator(DynTestEvaluator):
    def list_tests_for_job(self, job_name):
        return [
            ExecutedTest("TestPass", "pass", "42", "1", job_name, False),
            ExecutedTest("TestFail", "fail", "42", "1", job_name, False),
            ExecutedTest("TestFlaky", "fail", "42", "1", job_name, True),
            ExecutedTest("TestNotIndexed", "fail", "42", "1", job_name, False),
        ]


class TestDynTestEvaluator(unittest.TestCase):
    def setUp(self):
        self.executor = MagicMock()
        self.executor.commit_sha = "commit123"
        self.index = DynamicTestIndex({"job": {"pkg": ["TestPass", "TestFail", "TestFlaky"]}})
        self.executor.index.return_value = self.index
        self.telemetry = MagicMock()
        self.evaluator = _FakeEvaluator(
            MagicMock(), IndexKind.PACKAGE, self.executor, "42", telemetry_handler=self.telemetry
        )
        # the allow-failure fetch is networked; the evaluator tests are not
        # about it (see test_unreliable_jobs_fetch_fails_open)
        self.evaluator._unreliable_jobs = set()

    def test_initialize_success(self):
        self.assertTrue(self.evaluator.initialize())
        self.executor.init_index.assert_called_once_with()
        self.assertIs(self.evaluator.index, self.index)
        self.telemetry.send_event.assert_called_once()
        self.assertEqual(self.telemetry.send_event.call_args.args[0].alert_type, "success")

    def test_initialize_errors(self):
        for error, error_type in [
            (RuntimeError("No ancestor commit found for commit123"), "index_not_found"),
            (RuntimeError("Backend connection failed"), "index_initialization_failed"),
            (ValueError("Unexpected value"), "unexpected_error"),
        ]:
            with self.subTest(error_type=error_type):
                self.telemetry.reset_mock()
                self.executor.init_index.side_effect = error
                self.assertFalse(self.evaluator.initialize())
                event = self.telemetry.send_event.call_args.args[0]
                self.assertEqual(event.title, f"Dynamic Test Evaluator Error: {error_type}")
                self.assertIn("commit_sha:commit123", event.tags)

    def test_failed_telemetry_falls_back_to_console(self):
        self.executor.init_index.side_effect = RuntimeError("offline")
        self.telemetry.send_event.return_value = False
        with patch("builtins.print") as output:
            self.assertFalse(self.evaluator.initialize())
        self.assertTrue(any("offline" in str(call) for call in output.call_args_list))

    def test_misses_and_unindexed_tests(self):
        self.executor.tests_to_run_per_job.return_value = {"job": {"TestPass", "Unrelated"}}
        self.assertTrue(self.evaluator.initialize())
        result = self.evaluator.evaluate([])[0]
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})
        self.assertEqual(result.predicted_executed_tests, {"TestPass"})
        self.assertEqual(result.not_executed_failing_tests, {"TestFail"})

    def test_jobs_allowed_to_fail_never_miss(self):
        """Failing tests in jobs the pipeline allows to fail are not critical
        misses: GitLab ignores those jobs' result, so a failure there is
        known-unreliable."""
        self.executor.tests_to_run_per_job.return_value = {"job": set()}
        # the cached allow-failure set, as a fetch from GitLab would fill it
        self.evaluator._unreliable_jobs = {"job"}
        self.assertTrue(self.evaluator.initialize())
        result = self.evaluator.evaluate([])[0]
        self.assertEqual(result.not_executed_failing_tests, set())
        self.assertEqual(result.actual_executed_tests, {"TestPass", "TestFail", "TestFlaky"})

    @patch("tasks.libs.dynamic_test.evaluator.pipeline_jobs_allowed_to_fail")
    def test_unreliable_jobs_fetch_fails_open(self, allowed):
        """If the pipeline jobs cannot be fetched, nothing is filtered (the
        misses stay visible) instead of hiding them all."""
        evaluator = _FakeEvaluator(MagicMock(), IndexKind.PACKAGE, self.executor, "42", telemetry_handler=MagicMock())
        evaluator._unreliable_jobs = None  # not fetched yet
        allowed.side_effect = RuntimeError("gitlab down")
        self.assertEqual(evaluator.unreliable_jobs, set())
        # and the failure is cached, not retried per job
        allowed.side_effect = None
        self.assertEqual(evaluator.unreliable_jobs, set())

    @patch("tasks.libs.dynamic_test.evaluator.get_pipeline")
    def test_pipeline_jobs_allowed_to_fail(self, get_pipeline):
        """Allow-failure job names across the success and failed scopes."""
        pipeline = get_pipeline.return_value
        pipeline.jobs.list.side_effect = [
            iter(
                [
                    SimpleNamespace(name="new-e2e-ok", allow_failure=False),
                    SimpleNamespace(name="new-e2e-allowed", allow_failure=True),
                ]
            ),
            iter(
                [
                    SimpleNamespace(name="new-e2e-failed-allowed", allow_failure=True),
                    SimpleNamespace(name="new-e2e-failed", allow_failure=False),
                ]
            ),
        ]
        self.assertEqual(pipeline_jobs_allowed_to_fail("42"), {"new-e2e-allowed", "new-e2e-failed-allowed"})
        self.assertEqual([c.kwargs["scope"] for c in pipeline.jobs.list.call_args_list], ["success", "failed"])

    def test_summary_lists_the_missed_failing_tests(self):
        """The global summary warning names the skipped failing tests and their
        jobs: the per-job reports are long, the summary is what gets read."""
        self.executor.tests_to_run_per_job.return_value = {"job": set()}
        self.assertTrue(self.evaluator.initialize())
        with patch("builtins.print") as output:
            self.evaluator.print_summary(self.evaluator.evaluate([]))
        printed = [str(call) for call in output.call_args_list]
        self.assertTrue(any("1 failing tests would not have been executed" in p for p in printed))
        self.assertTrue(any("TestFail" in p and "job" in p for p in printed))

    def test_console_telemetry_never_publishes(self):
        with (
            patch("tasks.libs.dynamic_test.telemetry.send_event") as event,
            patch("tasks.libs.dynamic_test.telemetry.send_metrics") as metrics,
        ):
            self.evaluator.telemetry_handler = ConsoleTelemetryHandler()
            self.assertTrue(self.evaluator.initialize())
            self.executor.tests_to_run_per_job.return_value = {}
            self.evaluator.send_stats_to_datadog(self.evaluator.evaluate([]))
            event.assert_not_called()
            metrics.assert_not_called()


class TestDatadogDynTestEvaluator(unittest.TestCase):
    @staticmethod
    def event(name, status="pass", flaky=False, job_id="7", job_name="job"):
        return {
            "attributes": {
                "attributes": {
                    "test": {"name": name, "status": status, "agent_is_flaky_failure": flaky},
                    "ci": {"job": {"id": job_id, "name": job_name}, "pipeline": {"id": "42"}},
                }
            }
        }

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_query_environment_window_and_filtering(self, events):
        events.return_value = [
            self.event("TestPass"),
            self.event("TestFail", "fail"),
            self.event("TestSkip", "skip"),
            self.event("TestRoot/Subtest", "fail"),
            self.event("", "fail"),
            self.event("TestFlaky", "fail", "true"),
            self.event("TestFlakyBool", "fail", True),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(),
            IndexKind.JEV,
            MagicMock(),
            "42",
            telemetry_handler=MagicMock(),
        )
        tests = evaluator.list_tests_for_job('job: ["matrix"]')
        query, days = events.call_args.args
        self.assertIn("@ci.pipeline.id:42", query)
        self.assertIn(r'@ci.job.name:"job: [\"matrix\"]"', query)
        # Skipped tests are excluded by the query itself
        self.assertIn("-@test.status:skip", query)
        self.assertEqual(days, 3)

        self.assertEqual([test.name for test in tests], ["TestPass", "TestFail", "TestFlaky", "TestFlakyBool"])
        self.assertFalse(tests[1].unreliable_status)
        self.assertTrue(tests[2].unreliable_status)
        self.assertTrue(tests[3].unreliable_status)

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_passed_on_retry_counts_as_success(self, events):
        """A GitLab job retry reruns the whole test set: the failed attempt and
        the successful retry carry the same job name but different job ids.
        One success in the attempts means the test passed - it is not a
        critical miss."""
        events.return_value = [
            self.event("TestRetry", "fail", job_id="7"),  # first attempt
            self.event("TestRetry", "pass", job_id="8"),  # retried job
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(len(tests), 1)
        self.assertEqual(tests[0].status, "pass")

        # ...and a test that failed every attempt still fails
        events.return_value = [
            self.event("TestHardFail", "fail", job_id="7"),
            self.event("TestHardFail", "fail", job_id="8"),
        ]
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(len(tests), 1)
        self.assertEqual(tests[0].status, "fail")

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_pass_in_another_job_is_not_a_retry(self, events):
        """A pass in a DIFFERENT job (e.g. the same test selected by another
        e2e job) does not excuse this job's failure."""
        events.return_value = [
            self.event("TestDup", "fail"),
            self.event("TestDup", "pass", job_name="other-e2e-job"),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        fail = [t for t in tests if t.job_name == "job"][0]
        self.assertEqual(fail.status, "fail")

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_flaky_marking_on_any_attempt_wins(self, events):
        """CI Vis marks a flaky failure on the attempt it saw - keep the
        unreliable marking no matter which attempt carried it."""
        events.return_value = [
            self.event("TestFlakyRetry", "fail"),
            self.event("TestFlakyRetry", "fail", flaky=True, job_id="8"),
        ]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.JEV, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        tests = evaluator.list_tests_for_job("job")
        self.assertEqual(tests[0].status, "fail")
        self.assertTrue(tests[0].unreliable_status)

    @patch("tasks.libs.dynamic_test.evaluator.get_ci_test_events")
    def test_coverage_defaults(self, events):
        events.return_value = [self.event("TestFail", "fail")]
        evaluator = DatadogDynTestEvaluator(
            MagicMock(), IndexKind.FILE, MagicMock(), "42", telemetry_handler=MagicMock()
        )
        self.assertFalse(evaluator.list_tests_for_job("job")[0].unreliable_status)
        query, days = events.call_args.args
        self.assertEqual(days, 3)


class TestConsolidatedEvaluation(unittest.TestCase):
    def setUp(self):
        self.decoder = ApiClient(Configuration())
        self.addCleanup(self.decoder.close)

    def page(self, events, after=None, **kwargs):
        data = []
        for number, event in enumerate(events):
            data.append({'id': str(number), 'type': 'citest', **event})
        response = {'data': data, 'meta': {'status': 'done', 'page': {'after': after}}, **kwargs}
        return self.decoder.deserialize(json.dumps(response), (CIAppTestEventsResponse,), True)

    @staticmethod
    def evaluator(kind=IndexKind.JEV, jobs=None):
        jobs = jobs if jobs is not None else ['job', 'empty']
        index = DynamicTestIndex(
            {job: {'pkg': ['TestPass', 'TestFail', 'TestRetry', 'TestFlaky', 'TestNever']} for job in jobs}
        )
        executor = MagicMock()
        executor.tests_to_run_per_job.return_value = {job: {'TestPass', 'TestNever'} for job in jobs}
        evaluator = DatadogDynTestEvaluator(MagicMock(), kind, executor, '42', telemetry_handler=MagicMock())
        evaluator.index = index
        evaluator._unreliable_jobs = set()
        return evaluator

    def test_all_index_modes_keep_outputs_retries_empty_rows_and_telemetry(self):
        event = TestDatadogDynTestEvaluator.event
        pages = [
            [event('TestRetry', 'fail', job_id='7'), event('TestFail', 'fail'), event('TestFlaky', 'fail')],
            [
                event('TestRetry', 'pass', job_id='8'),
                event('TestFlaky', 'fail', True),
                event('TestPass'),
                event('TestRoot/Subtest', 'fail'),
            ],
        ]
        for kind in [IndexKind.JEV, IndexKind.PACKAGE, IndexKind.FILE, IndexKind.DIFFED_PACKAGE]:
            for allowed in [set(), {'job'}]:
                with self.subTest(kind=kind, allowed=allowed):
                    evaluator = self.evaluator(kind, ['empty', 'job', 'never-ran'])
                    evaluator._unreliable_jobs = allowed
                    with patch.object(
                        CIVisibilityTestsApi,
                        'search_ci_app_test_events',
                        side_effect=[self.page(pages[0], 'next'), self.page(pages[1])],
                    ) as post:
                        results = evaluator.evaluate(['changed.py'])
                    self.assertEqual(post.call_count, 2)
                    self.assertEqual([result.job_name for result in results], ['empty', 'job', 'never-ran'])
                    self.assertEqual(results[0].actual_executed_tests, set())
                    self.assertEqual(results[2].predicted_executed_tests, {'TestPass', 'TestNever'})
                    self.assertEqual(
                        results[1].actual_executed_tests, {'TestPass', 'TestFail', 'TestRetry', 'TestFlaky'}
                    )
                    self.assertEqual(results[1].not_executed_failing_tests, set() if allowed else {'TestFail'})
                    evaluator.executor.tests_to_run_per_job.assert_called_once_with(['changed.py'])
                    evaluator.send_stats_to_datadog(results)
                    actual_counts = [
                        call.args[1]
                        for call in evaluator.telemetry_handler.count.call_args_list
                        if call.args[0].endswith('.actual_executed_tests')
                    ]
                    self.assertEqual(actual_counts, [0, 4, 0])
                    stream = io.StringIO()
                    with redirect_stdout(stream):
                        evaluator.print_summary(results)
                    self.assertIn('actual=4 predicted=6', stream.getvalue())
                    self.assertEqual('1 failing tests would not have been executed' in stream.getvalue(), not allowed)

    def test_reduced_order_and_first_attempt_metadata_survive_page_boundaries(self):
        event = TestDatadogDynTestEvaluator.event
        evaluator = self.evaluator(jobs=['job', 'other'])
        pages = [
            self.page([event('TestRetry', 'fail', job_id='7'), event('TestRetry', 'fail', job_name='other')], 'next'),
            self.page([event('TestPass'), event('TestRetry', 'pass', job_id='8')]),
        ]
        with patch.object(CIVisibilityTestsApi, 'search_ci_app_test_events', side_effect=pages):
            rows = dict(evaluator._iter_tests_per_job(['job', 'other']))
        self.assertEqual(
            rows['job'],
            [
                ExecutedTest('TestRetry', 'pass', '42', '7', 'job', False),
                ExecutedTest('TestPass', 'pass', '42', '7', 'job', False),
            ],
        )
        self.assertEqual(rows['other'][0].status, 'fail')

    def test_queries_partition_exact_names_in_order_and_enforce_both_limits(self):
        evaluator = self.evaluator()
        matrix = 'job: ["matrix", \\windows]'
        names = [matrix, *[f'job-{number}' for number in range(100)]]
        queries = evaluator._test_event_queries(names)
        self.assertEqual([query.count('@ci.job.name:') for query in queries], [50, 50, 1])
        self.assertIn(r'@ci.job.name:"job: [\"matrix\", \\windows]"', queries[0])
        self.assertEqual(sum(query.count('@ci.job.name:') for query in queries), len(names))
        for query in queries:
            self.assertIn(
                'env:prod @ci.pipeline.name:DataDog/datadog-agent @ci.pipeline.id:42 -@test.status:skip (', query
            )
            self.assertLessEqual(len(json.dumps(query).encode()), evaluator._MAX_QUERY_BYTES)
        long_names = [f'{number}-' + '\u2603' * 250 for number in range(8)]
        queries = evaluator._test_event_queries(long_names)
        self.assertGreater(len(queries), 1)
        self.assertTrue(all(len(json.dumps(query).encode()) <= evaluator._MAX_QUERY_BYTES for query in queries))
        with patch.object(CIVisibilityTestsApi, 'search_ci_app_test_events') as post:
            with self.assertRaisesRegex(ValueError, 'predicate exceeds'):
                list(evaluator._iter_tests_per_job(['job', '\u2603' * 2000]))
            post.assert_not_called()

    def test_fixed_window_across_pages_batches_and_fresh_window_on_repeat(self):
        evaluator = self.evaluator(jobs=[f'job-{number}' for number in range(51)])
        end = datetime(2026, 11, 8, 12, tzinfo=timezone.utc)
        with (
            patch('tasks.libs.dynamic_test.evaluator.datetime') as clock,
            patch.object(
                CIVisibilityTestsApi,
                'search_ci_app_test_events',
                side_effect=[self.page([], 'next'), self.page([]), self.page([]), self.page([]), self.page([])],
            ) as post,
        ):
            clock.now.side_effect = [end, end + timedelta(seconds=1)]
            evaluator.evaluate([])
            evaluator.evaluate([])
        bodies = [call.kwargs['body'].to_dict() for call in post.call_args_list]
        for body in bodies[:3]:
            self.assertEqual(body['filter']['_from'], (end - timedelta(days=3)).isoformat())
            self.assertEqual(body['filter']['to'], end.isoformat())
        self.assertEqual(bodies[3]['filter']['to'], (end + timedelta(seconds=1)).isoformat())

    def test_repeated_evaluation_and_direct_lookup_do_not_reuse_events(self):
        evaluator = self.evaluator()
        event = TestDatadogDynTestEvaluator.event
        with patch.object(
            CIVisibilityTestsApi,
            'search_ci_app_test_events',
            side_effect=[self.page([event('TestPass')]), self.page([event('TestFail', 'fail')])],
        ):
            self.assertEqual(evaluator.evaluate([])[0].actual_executed_tests, {'TestPass'})
            self.assertEqual(evaluator.evaluate([])[0].not_executed_failing_tests, {'TestFail'})
        with patch(
            'tasks.libs.dynamic_test.evaluator.get_ci_test_events', side_effect=[[event('TestPass')], []]
        ) as direct:
            self.assertEqual(evaluator.list_tests_for_job('job')[0].name, 'TestPass')
            self.assertEqual(evaluator.list_tests_for_job('job'), [])
            self.assertEqual(direct.call_count, 2)

    def test_incomplete_retrieval_cannot_return_results_or_publish_metrics(self):
        evaluator = self.evaluator()
        with patch.object(
            CIVisibilityTestsApi,
            'search_ci_app_test_events',
            side_effect=[
                self.page([TestDatadogDynTestEvaluator.event('TestPass')], 'next'),
                RuntimeError('backend failed'),
            ],
        ):
            with self.assertRaisesRegex(RuntimeError, 'backend failed'):
                evaluator.evaluate([])
        evaluator.telemetry_handler.count.assert_not_called()
        evaluator.telemetry_handler.send_event.assert_not_called()

    def test_unexpected_job_or_pipeline_is_an_error_and_closes_stream(self):
        for wrong in ['job', 'pipeline']:
            event = TestDatadogDynTestEvaluator.event('TestPass', job_name='unexpected' if wrong == 'job' else 'job')
            if wrong == 'pipeline':
                event['attributes']['attributes']['ci']['pipeline']['id'] = 'other'
            closed = []

            def events(*args, event=event, closed=closed):
                try:
                    yield event
                finally:
                    closed.append(True)

            with self.subTest(wrong=wrong), patch('tasks.libs.dynamic_test.evaluator.iter_ci_test_events', events):
                with self.assertRaises(ValueError):
                    self.evaluator().evaluate([])
                self.assertEqual(closed, [True])

    def test_no_indexed_jobs_does_not_open_a_client(self):
        with patch('tasks.libs.dynamic_test.evaluator.iter_ci_test_events') as events:
            self.assertEqual(self.evaluator(jobs=[]).evaluate([]), [])
            events.assert_not_called()

    def test_streaming_reducer_keeps_the_first_seen_metadata_at_equal_times(self):
        event = TestDatadogDynTestEvaluator.event
        events = [event('TestRetry', 'fail', job_id='7'), event('TestRetry', 'pass', job_id='8')]
        for row in events:
            row['attributes']['attributes']['start'] = 1
        reduced = executed_tests_from_events(iter(events))
        self.assertEqual(reduced, [ExecutedTest('TestRetry', 'pass', '42', '7', 'job', False)])

    def test_window_boundaries_and_late_ingestion_do_not_create_a_cache_or_poll(self):
        evaluator = self.evaluator(jobs=['job'])
        end = datetime(2026, 11, 8, 12, tzinfo=timezone.utc)
        start = end - timedelta(days=3)
        available = [
            ('TestPass', start),
            ('TestRetry', end),
            ('TestFail', start - timedelta(microseconds=1)),
            ('TestFlaky', end + timedelta(microseconds=1)),
        ]

        def search(*, body):
            # Model the service's inclusive time filter. The client must send the
            # same absolute bounds, including when older events arrive later.
            lower = datetime.fromisoformat(body.filter._from)
            upper = datetime.fromisoformat(body.filter.to)
            return self.page(
                [
                    TestDatadogDynTestEvaluator.event(name)
                    for name, timestamp in available
                    if lower <= timestamp <= upper
                ]
            )

        with (
            patch('tasks.libs.dynamic_test.evaluator.datetime') as clock,
            patch.object(CIVisibilityTestsApi, 'search_ci_app_test_events', side_effect=search) as post,
        ):
            clock.now.return_value = end
            first = evaluator.evaluate([])[0]
            available.append(('TestNever', start + timedelta(hours=1)))
            second = evaluator.evaluate([])[0]
        self.assertEqual(first.actual_executed_tests, {'TestPass', 'TestRetry'})
        self.assertEqual(second.actual_executed_tests, {'TestPass', 'TestRetry', 'TestNever'})
        self.assertEqual(post.call_count, 2)
