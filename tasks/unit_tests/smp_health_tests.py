import copy
import json
import os
import shutil
import sqlite3
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path
from unittest import mock

import yaml

from tasks.libs.smp.experiment_health import (
    SCHEMA,
    SERVICE,
    add_replicate_peaks,
    build_payload,
    ci_context,
    expected_experiments,
    render_artifacts,
    summarize,
)


def report_fixture():
    return {
        '$schema': SCHEMA,
        'job': {
            'id': 'smp-run-1',
            'baseline_sha': 'baseline',
            'comparison_sha': 'comparison',
            'metrics_query_start': 100,
            'metrics_query_end': 200,
            'tolerances': {'p_value': 0.1, 'effect_size': 5.0, 'coefficient_of_variation_limit': 0.1},
        },
        'experiments': [
            {
                'name': 'advisory',
                'erratic': True,
                'optimization_goal': {
                    'goal': 'cpu',
                    'trial_count': 1,
                    'max_trial_count': 1,
                    'result': {
                        'percent_change': 10.0,
                        'confidence_interval': {'lower': 8.0, 'upper': 12.0},
                        'confidence_percent': 90.0,
                        'is_significant_change': True,
                        'is_improvement': False,
                        'is_erratic': False,
                    },
                },
                'bounds_checks': [
                    {
                        'name': 'rss',
                        'series': 'total_rss_bytes',
                        'unit': 'bytes',
                        'lower_bound': None,
                        'upper_bound': 100.0,
                        'pass_count': 1,
                        'total_count': 2,
                        'min_observed': 90.0,
                        'max_observed': 110.0,
                    }
                ],
                'quantile_checks': [],
            }
        ],
        'analysis_errors': [],
        'failed_replicates': [],
    }


def context_fixture(pipeline='100', job='200', number=42):
    return ci_context(
        {'CI_COMMIT_BRANCH': 'feature', 'CI_DEFAULT_BRANCH': 'main', 'PARENT_PIPELINE_ID': pipeline, 'CI_JOB_ID': job},
        {'status': 'resolved', 'number': number, 'url': f'https://github.com/DataDog/datadog-agent/pull/{number}'},
    )


def replica_fixture(check='rss'):
    return {
        'job_id': 'smp-run-1',
        'experiments': {
            'advisory': {
                'bounds_checks': {
                    check: {
                        'results': {
                            'baseline': [
                                {'replicate_idx': 0, 'passed': True, 'min_observed': 1.0, 'max_observed': 2.0}
                            ],
                            'comparison': [
                                {'replicate_idx': 0, 'passed': True, 'min_observed': 20.0, 'max_observed': 95.0},
                                {'replicate_idx': 1, 'passed': False, 'min_observed': 90.0, 'max_observed': 110.0},
                            ],
                        }
                    }
                }
            }
        },
    }


class TestReplicatePeaks(unittest.TestCase):
    def test_peak_range_uses_comparison_maxima_and_preserves_outcomes(self):
        report = report_fixture()
        report['experiments'][0]['bounds_checks'][0]['min_observed'] = 20.0
        payload = build_payload(report, context_fixture(), {})
        add_replicate_peaks(payload, replica_fixture())
        bounds = payload['evaluations'][0]
        self.assertEqual(bounds['outcome'], 'fail')
        self.assertEqual(bounds['tags']['smp.report_version'], '2')
        self.assertEqual(bounds['tags']['smp.bounds.min_observed'], '20.0')
        self.assertEqual(bounds['tags']['smp.bounds.lowest_replicate_peak'], '95.0')
        self.assertEqual(bounds['tags']['smp.bounds.highest_replicate_peak'], '110.0')
        self.assertEqual(bounds['tags']['smp.bounds.peak_summary_status'], 'complete')
        self.assertEqual(json.loads(bounds['details'])['lowest_replicate_peak'], 95.0)
        self.assertEqual(payload['suites'][0]['evaluations'][0]['tags'], bounds['tags'])

    def test_inconsistent_replica_reports_fail_closed(self):
        for case in ['job', 'count', 'duplicate', 'outcome', 'maximum', 'boolean', 'infinite']:
            with self.subTest(case=case):
                legacy = replica_fixture()
                rows = legacy['experiments']['advisory']['bounds_checks']['rss']['results']['comparison']
                if case == 'job':
                    legacy['job_id'] = 'other-job'
                elif case == 'count':
                    rows.pop()
                elif case == 'duplicate':
                    rows[1]['replicate_idx'] = 0
                elif case == 'outcome':
                    rows[1]['passed'] = True
                else:
                    rows[1]['max_observed'] = {'maximum': 120, 'boolean': True, 'infinite': float('inf')}[case]
                with self.assertRaises(ValueError):
                    add_replicate_peaks(build_payload(report_fixture(), context_fixture(), {}), legacy)

    def test_missing_peak_does_not_create_a_zero_or_hide_a_failure(self):
        legacy = replica_fixture()
        legacy['experiments']['advisory']['bounds_checks']['rss']['results']['comparison'][0]['max_observed'] = None
        payload = build_payload(report_fixture(), context_fixture(), {})
        add_replicate_peaks(payload, legacy)
        bounds = payload['evaluations'][0]
        self.assertEqual(bounds['outcome'], 'fail')
        self.assertEqual(bounds['tags']['smp.bounds.peak_summary_status'], 'unavailable')
        self.assertNotIn('smp.bounds.lowest_replicate_peak', bounds['tags'])

    def test_zero_is_a_valid_peak(self):
        report = report_fixture()
        report['experiments'][0]['bounds_checks'][0].update(pass_count=2, min_observed=0.0, max_observed=0.0)
        legacy = replica_fixture()
        for row in legacy['experiments']['advisory']['bounds_checks']['rss']['results']['comparison']:
            row.update(passed=True, min_observed=0.0, max_observed=0.0)
        payload = build_payload(report, context_fixture(), {})
        add_replicate_peaks(payload, legacy)
        self.assertEqual(payload['evaluations'][0]['tags']['smp.bounds.lowest_replicate_peak'], '0.0')
        self.assertEqual(payload['evaluations'][0]['tags']['smp.bounds.peak_summary_status'], 'complete')

    def test_missing_check_still_has_an_error_without_peak_values(self):
        report = report_fixture()
        report['experiments'][0]['bounds_checks'] = []
        payload = build_payload(report, context_fixture(), {'advisory': {'bounds': ['rss']}})
        add_replicate_peaks(payload, replica_fixture())
        self.assertEqual(payload['evaluations'][0]['outcome'], 'error')
        self.assertEqual(payload['evaluations'][0]['tags']['smp.bounds.peak_summary_status'], 'unavailable')


class TestHealthOutcomes(unittest.TestCase):
    def payload(self, report=None, expected=None):
        return build_payload(report or report_fixture(), context_fixture(), expected or {})

    def test_advisory_failures_are_not_hidden(self):
        rows = self.payload()['evaluations']
        self.assertEqual([row['outcome'] for row in rows], ['fail', 'fail'])
        self.assertTrue(all(row['tags']['smp.ignores_regressions'] == 'true' for row in rows))
        self.assertTrue(all(row['tags']['smp.quality_gate_bounds'] == 'false' for row in rows))

    def test_goal_direction_uses_analysis_flags(self):
        for significant in [False, True]:
            for improvement in [False, True]:
                for delta in [-10.0, 10.0]:
                    with self.subTest(significant=significant, improvement=improvement, delta=delta):
                        report = report_fixture()
                        result = report['experiments'][0]['optimization_goal']['result']
                        result.update(
                            is_significant_change=significant, is_improvement=improvement, percent_change=delta
                        )
                        goal = self.payload(report)['evaluations'][1]
                        self.assertEqual(goal['outcome'], 'fail' if significant and not improvement else 'pass')

    def test_bounds_zero_and_invalid_counts_are_errors(self):
        for passed, total, outcome in [
            (2, 2, 'pass'),
            (1, 2, 'fail'),
            (0, 0, 'error'),
            (-1, 2, 'error'),
            (3, 2, 'error'),
            (True, 2, 'error'),
        ]:
            with self.subTest(passed=passed, total=total):
                report = report_fixture()
                report['experiments'][0]['bounds_checks'][0].update(pass_count=passed, total_count=total)
                self.assertEqual(self.payload(report)['evaluations'][0]['outcome'], outcome)

    def test_missing_selected_experiment_and_check_are_errors(self):
        expected = {'missing': {'bounds': ['memory'], 'optimization_goal': 'cpu', 'erratic': False}}
        rows = [row for row in self.payload(expected=expected)['evaluations'] if row['experiment'] == 'missing']
        self.assertEqual({row['kind'] for row in rows}, {'bounds', 'optimization_goal', 'execution'})
        self.assertTrue(all(row['outcome'] == 'error' for row in rows))
        report = report_fixture()
        report['experiments'][0]['bounds_checks'] = []
        rows = self.payload(report, {'advisory': {'bounds': ['rss']}})['evaluations']
        self.assertEqual(rows[0]['outcome'], 'error')

    def test_bounds_only_experiment_has_no_synthetic_goal(self):
        report = report_fixture()
        report['experiments'][0]['optimization_goal'] = None
        self.assertEqual([row['kind'] for row in self.payload(report)['evaluations']], ['bounds'])

    def test_missing_goal_keeps_identity(self):
        report = report_fixture()
        original = self.payload(report)['evaluations'][1]
        report['experiments'][0]['optimization_goal'].update(goal=None, result=None)
        missing = self.payload(report)['evaluations'][1]
        self.assertEqual(missing['outcome'], 'error')
        self.assertEqual(original['tags']['smp.evaluation_id'], missing['tags']['smp.evaluation_id'])

    def test_all_diagnostic_failures_are_preserved(self):
        report = report_fixture()
        report['experiments'][0]['optimization_goal']['result']['is_erratic'] = True
        error = {
            'experiment': 'advisory',
            'analysis_type': 'quantile_bounds',
            'check_name': 'rss',
            'message': 'bad data',
        }
        report['analysis_errors'] = [error, error]
        report['failed_replicates'] = [
            {
                'experiment': 'advisory',
                'variant': 'comparison',
                'replicate_type': 'non_profiling',
                'failure': {'kind': 'crashed', 'exit_code': 1},
                'is_dead': False,
                'replicates': [{'replicate_index': 0, 'count': 1}],
            }
        ]
        payload = self.payload(report)
        self.assertEqual(payload['errors'], 3)
        self.assertEqual(payload['failures'], 2)
        diagnostic = next(row for row in payload['evaluations'] if row['kind'] == 'execution')
        self.assertIn('"is_dead": false', diagnostic['details'])
        self.assertEqual(len({row['tags']['smp.evaluation_id'] for row in payload['evaluations']}), 5)

    def test_duplicates_and_unknown_schema_fail_closed(self):
        report = report_fixture()
        report['$schema'] = 'v2'
        with self.assertRaises(ValueError):
            self.payload(report)
        report = report_fixture()
        report['experiments'].append(copy.deepcopy(report['experiments'][0]))
        with self.assertRaises(ValueError):
            self.payload(report)


class TestHealthContext(unittest.TestCase):
    def test_main_wins_over_stale_pr_context_and_secrets_are_absent(self):
        env = {'CI_COMMIT_BRANCH': 'main', 'CI_DEFAULT_BRANCH': 'main', 'DATADOG_API_KEY': 'secret'}
        context = ci_context(env, {'status': 'resolved', 'number': 42})
        self.assertEqual(context['smp.context'], 'main')
        self.assertNotIn('smp.pr_number', context)
        self.assertNotIn('secret', json.dumps(context))

    def test_unknown_or_ambiguous_pr_is_not_invented(self):
        for status in ['absent', 'ambiguous', 'unavailable']:
            context = ci_context({'CI_COMMIT_BRANCH': 'feature'}, {'status': status})
            self.assertEqual(context['smp.context'], 'other')

    def test_selected_inventory_ignores_unselected_experiments(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'chosen').mkdir()
            (root / 'chosen/experiment.yaml').write_text(
                'optimization_goal: memory\nchecks:\n  - name: rss\n    bounds: {upper_bound: 2}\n'
            )
            (root / 'unselected').mkdir()
            (root / 'unselected/experiment.yaml').write_text('not: valid: yaml')
            result = expected_experiments(root, [{'name': 'chosen', 'path': 'chosen'}])
            self.assertEqual(result['chosen']['bounds'], ['rss'])
            with self.assertRaises(ValueError):
                expected_experiments(root, [{'name': 'escape', 'path': '../escape'}])


class TestHealthViews(unittest.TestCase):
    def test_reruns_affect_history_but_not_latest_and_duplicates_do_not_inflate(self):
        report = report_fixture()
        old = build_payload(report, context_fixture('9', '99'), {})
        report['job']['id'] = 'smp-run-2'
        report['experiments'][0]['bounds_checks'][0]['pass_count'] = 2
        report['experiments'][0]['optimization_goal']['result']['is_significant_change'] = False
        new = build_payload(report, context_fixture('10', '100'), {})
        summary = summarize([new, old, new])
        self.assertEqual(len(summary['all_evaluations']['evaluations']), 4)
        self.assertEqual(len(summary['latest_per_pr']['evaluations']), 2)
        self.assertTrue(all(row['pass_rate'] == 100 for row in summary['latest_per_pr']['counts']))
        self.assertTrue(all(row['pass_rate'] == 50 for row in summary['all_evaluations']['counts']))

    def test_latest_selects_whole_run_so_recovered_diagnostics_clear(self):
        report = report_fixture()
        report['analysis_errors'] = [
            {'experiment': 'advisory', 'analysis_type': 'quantile_bounds', 'check_name': 'rss', 'message': 'error'}
        ]
        old = build_payload(report, context_fixture(), {})
        report['job']['id'] = 'retried-run'
        report['analysis_errors'] = []
        new = build_payload(report, context_fixture(job='201'), {})
        summary = summarize([old, new])
        self.assertFalse(any(row['kind'] == 'analysis' for row in summary['latest_per_pr']['evaluations']))
        self.assertTrue(any(row['kind'] == 'analysis' for row in summary['all_evaluations']['evaluations']))

    def test_main_other_and_other_prs_do_not_replace_a_pr(self):
        pr = build_payload(report_fixture(), context_fixture(), {})
        other_pr = build_payload(report_fixture(), context_fixture('101', '201', 43), {})
        main = build_payload(
            report_fixture(),
            ci_context({'CI_COMMIT_BRANCH': 'main', 'CI_DEFAULT_BRANCH': 'main', 'CI_JOB_ID': '202'}, {}),
            {},
        )
        result = summarize([pr, other_pr, main])
        self.assertEqual(len(result['latest_per_pr']['evaluations']), 4)
        self.assertEqual(len(result['all_evaluations']['evaluations']), 6)
        self.assertEqual(summarize([pr], experiment='absent')['all_evaluations']['counts'], [])

    def test_errors_do_not_count_as_passes_or_performance_failures(self):
        report = report_fixture()
        report['experiments'][0]['optimization_goal']['result'] = None
        result = summarize([build_payload(report, context_fixture(), {})])
        goal = next(row for row in result['all_evaluations']['counts'] if row['kind'] == 'optimization_goal')
        self.assertEqual(
            goal,
            {
                'context': 'pr',
                'experiment': 'advisory',
                'kind': 'optimization_goal',
                'pass': 0,
                'fail': 0,
                'error': 1,
                'pass_rate': None,
            },
        )

    def test_conflicting_duplicate_is_rejected(self):
        payload = build_payload(report_fixture(), context_fixture(), {})
        different = copy.deepcopy(payload)
        different['evaluations'][0]['outcome'] = 'pass'
        with self.assertRaises(ValueError):
            summarize([payload, different])

    def test_latest_sql_matches_whole_run_selection(self):
        sql = Path('test/regression/reporting/latest-pr.sql').read_text()
        # SQLite checks the reduction independently from the Python selector.
        reduction = sql[sql.index('runs AS (') :]
        db = sqlite3.connect(':memory:')
        db.execute(
            'CREATE TABLE events (pr_number TEXT, experiment TEXT, check_kind TEXT, check_name TEXT, outcome TEXT, parent_pipeline_id INT, ci_job_id INT, analysis_end INT, smp_job_id TEXT)'
        )
        db.executemany(
            'INSERT INTO events VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)',
            [
                ('42', 'cpu', 'optimization_goal', 'optimization_goal', 'fail', 9, 99, 300, 'old'),
                ('42', 'cpu', 'analysis', 'stability', 'error', 9, 99, 300, 'old'),
                ('42', 'cpu', 'optimization_goal', 'optimization_goal', 'pass', 10, 100, 200, 'new'),
                ('43', 'cpu', 'optimization_goal', 'optimization_goal', 'error', 8, 98, 100, 'other'),
            ],
        )
        result = db.execute('WITH ' + reduction).fetchall()
        self.assertEqual(sorted((row[0], row[4]) for row in result), [('42', 'pass'), ('43', 'error')])


class TestHealthRendering(unittest.TestCase):
    @unittest.skipUnless(shutil.which('smp'), 'Install SMP v0.29.1 to run the renderer integration test')
    def test_real_renderer_escapes_xml_and_preserves_observed_failures(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = root / 'config'
            (config / 'advisory').mkdir(parents=True)
            (config / 'advisory/experiment.yaml').write_text(
                'optimization_goal: cpu\nerratic: true\nchecks:\n  - name: "rss & <limit>"\n    bounds: {series: total_rss_bytes, upper_bound: 100}\n'
            )
            (config / 'selection.yaml').write_text('always:\n  - advisory\n')
            report = report_fixture()
            report['experiments'][0]['bounds_checks'][0]['name'] = 'rss & <limit>'
            report_path = root / 'report.v1.json'
            report_path.write_text(json.dumps(report))
            (root / 'report.json').write_text(json.dumps(replica_fixture('rss & <limit>')))
            actual = render_artifacts(
                shutil.which('smp'), report_path, root / 'health', str(config), root / 'absent-pr.json', {}
            )
            self.assertEqual(actual, {'tests': 2, 'failures': 2, 'errors': 0})
            tree = ET.parse(root / 'health/experiment-health.junit.xml')
            self.assertEqual(tree.find('.//testcase').attrib['name'], 'bounds::rss & <limit>')
            properties = {p.attrib['name']: p.attrib['value'] for p in tree.findall('.//testcase/properties/property')}
            self.assertEqual(properties['dd_tags[smp.bounds.lowest_replicate_peak]'], '95.0')
            self.assertEqual(properties['dd_tags[smp.bounds.highest_replicate_peak]'], '110.0')
            self.assertEqual(json.loads((root / 'health/upload-plan.json').read_text())['service'], SERVICE)
            self.assertFalse(json.loads((root / 'health/upload-plan.json').read_text())['enabled'])

    @mock.patch('tasks.libs.smp.experiment_health.subprocess.run', side_effect=RuntimeError('render failed'))
    def test_failed_render_removes_stale_xml(self, _run):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'experiment-health.junit.xml'
            path.write_text('stale')
            with self.assertRaises(RuntimeError):
                render_artifacts('smp', 'missing', tmp, 'missing', 'missing')
            self.assertFalse(path.exists())

    def test_reporting_failure_is_nonblocking_and_metal_lane_skips_it(self):
        class Loader(yaml.SafeLoader):
            pass

        Loader.add_constructor('!reference', lambda loader, node: loader.construct_sequence(node))
        pipeline = yaml.load(Path('.gitlab/childs/smp-regression-child-pipeline.yml').read_text(), Loader=Loader)
        script = pipeline['.single-machine-performance-run_regression_detector']['script']
        reporting = next(
            command for command in script if isinstance(command, str) and 'dda inv smp.experiment-health' in command
        )
        for selection, status in [('true', True), ('false', False)]:
            with self.subTest(selection=selection), tempfile.TemporaryDirectory() as tmp:
                result = subprocess.run(
                    ['bash', '-e', '-c', 'dda() { return 1; }\n' + reporting],
                    cwd=tmp,
                    env={**os.environ, 'SELECT_EXPERIMENTS': selection},
                    capture_output=True,
                )
                self.assertEqual(result.returncode, 0)
                self.assertEqual((Path(tmp) / 'outputs/experiment-health/render-status.json').exists(), status)

    def test_pilot_upload_is_manual_and_uses_only_the_new_report(self):
        class Loader(yaml.SafeLoader):
            pass

        Loader.add_constructor('!reference', lambda loader, node: loader.construct_sequence(node))
        pipeline = yaml.load(Path('.gitlab/childs/smp-regression-child-pipeline.yml').read_text(), Loader=Loader)
        job = pipeline['single-machine-performance-experiment-health-upload']
        self.assertEqual(job['rules'], [{'when': 'manual'}])
        self.assertTrue(job['allow_failure'])
        self.assertEqual(job['retry'], 0)
        self.assertEqual(job['needs'], [{'job': 'single-machine-performance-regression_detector', 'artifacts': True}])
        self.assertEqual(job['variables']['DD_CIVISIBILITY_LOGS_ENABLED'], 'false')
        command = job['script'][-1]
        result = subprocess.run(
            ['bash', '-eu', '-c', 'datadog-ci() { printf "%s\\n" "$@"; }\n' + command],
            env={**os.environ, 'CI_JOB_ID': '123'},
            capture_output=True,
            text=True,
            check=True,
        )
        arguments = result.stdout.splitlines()
        self.assertEqual(arguments[arguments.index('--service') + 1], SERVICE)
        self.assertEqual(arguments[arguments.index('--env') + 1], 'ci-pilot')
        self.assertIn('smp.pilot:true', arguments)
        self.assertEqual(arguments[-1], 'outputs/experiment-health/experiment-health.junit.xml')
        self.assertNotIn('--logs', arguments)


if __name__ == '__main__':
    unittest.main()
