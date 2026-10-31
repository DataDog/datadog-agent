"""Create local SMP health artifacts. This module has no upload operation."""

import hashlib
import json
import os
import subprocess
import xml.etree.ElementTree as ET
from collections import defaultdict
from pathlib import Path

import yaml

SERVICE = 'datadog-agent-smp-experiment-health'
SCHEMA = 'tag:datadoghq.dev,2026-09-10:single-machine-performance/schemas/report/v1'
TEMPLATE = Path('test/regression/reporting/experiment-health.junit.xml.j2')


def read_json(path):
    def reject_constant(value):
        raise ValueError(f'Invalid JSON number: {value}')

    return json.loads(Path(path).read_text(encoding='utf-8'), parse_constant=reject_constant)


def write_json(path, value):
    Path(path).write_text(json.dumps(value, indent=2, sort_keys=True, allow_nan=False) + '\n', encoding='utf-8')


def ci_context(env, pr):
    """Copy only report metadata. Never copy the complete CI environment."""
    branch = env.get('CI_COMMIT_BRANCH', '')
    context = 'other'
    if branch and branch == env.get('CI_DEFAULT_BRANCH'):
        context = 'main'
    elif pr.get('status') == 'resolved':
        context = 'pr'

    def sequence(key, fallback=''):
        value = env.get(key) or fallback
        if value and not value.isdecimal():
            raise ValueError(f'{key} must contain a numeric CI identifier')
        return value or '0'

    tags = {
        'smp.report_kind': 'experiment_health',
        'smp.report_version': '1',
        'smp.context': context,
        'smp.branch': branch,
        'smp.pr_context_status': 'not_applicable' if context == 'main' else pr.get('status', 'unavailable'),
        'smp.parent_pipeline_id': sequence('PARENT_PIPELINE_ID', env.get('CI_PIPELINE_ID', '')),
        'smp.ci_job_id': sequence('CI_JOB_ID'),
        'smp.ci_job_url': env.get('CI_JOB_URL', ''),
    }
    if context == 'pr':
        number = pr.get('number')
        if not isinstance(number, int) or isinstance(number, bool) or number <= 0:
            raise ValueError('Resolved PR context needs a positive PR number')
        tags['smp.pr_number'] = str(number)
        tags['smp.pr_url'] = pr['url']
    return tags


def expected_experiments(config_dir, selected):
    """Read only selected definitions, so absent results cannot look healthy."""
    root = Path(config_dir).resolve()
    expected = {}
    for item in selected:
        path = (root / item['path'] / 'experiment.yaml').resolve()
        if not path.is_relative_to(root):
            raise ValueError('Selected experiment path is outside the configuration directory')
        config = yaml.safe_load(path.read_text(encoding='utf-8'))
        name = item['name']
        if name in expected:
            raise ValueError(f'Duplicate selected experiment: {name}')
        expected[name] = {
            'erratic': config.get('erratic', False),
            'optimization_goal': config.get('optimization_goal'),
            'bounds': [check['name'] for check in config.get('checks', []) if 'bounds' in check],
        }
    return expected


def build_payload(report, context, expected):
    if report['$schema'] != SCHEMA:
        raise ValueError('Unsupported SMP report schema')
    job = report['job']
    common = {
        **context,
        'smp.job_id': job['id'],
        'smp.baseline_sha': job['baseline_sha'],
        'smp.comparison_sha': job['comparison_sha'],
        'smp.analysis_end': str(job['metrics_query_end']),
        'smp.effect_size_threshold': str(job['tolerances']['effect_size']),
        'smp.p_value_threshold': str(job['tolerances']['p_value']),
    }
    experiments = {}
    for experiment in report['experiments']:
        name = experiment['name']
        if name in experiments:
            raise ValueError(f'Duplicate experiment result: {name}')
        experiments[name] = experiment
    evaluations = []

    def add(name, kind, check, outcome, message, details, attributes=None):
        config = expected.get(name, {})
        exp = experiments.get(name, {})
        tags = {
            **common,
            'smp.experiment': name,
            'smp.check_kind': kind,
            'smp.check_name': check,
            'smp.outcome': outcome,
            'smp.ignores_regressions': str(exp.get('erratic', config.get('erratic', False))).lower(),
            'smp.quality_gate_bounds': str(kind == 'bounds' and name.startswith('quality_gate_')).lower(),
            **(attributes or {}),
        }
        identity = [common['smp.ci_job_id'], job['id'], name, kind, check]
        tags['smp.evaluation_id'] = hashlib.sha256(json.dumps(identity).encode()).hexdigest()
        evaluations.append(
            {
                'experiment': name,
                'kind': kind,
                'name': f'{kind}::{check}',
                'outcome': outcome,
                'message': message,
                'details': json.dumps(details, sort_keys=True, allow_nan=False),
                'tags': tags,
            }
        )

    for name in sorted(expected.keys() | experiments.keys()):
        exp = experiments.get(name, {})
        config = expected.get(name, {})
        bounds = {}
        for check in exp.get('bounds_checks', []):
            if check['name'] in bounds:
                raise ValueError(f'Duplicate bounds result: {name}/{check["name"]}')
            bounds[check['name']] = check
        for check_name in sorted(set(config.get('bounds', [])) | bounds.keys()):
            check = bounds.get(check_name)
            outcome = 'error'
            message = 'Bounds result is missing or has invalid replica counts'
            if check:
                passed, total = check['pass_count'], check['total_count']
                if type(passed) is int and type(total) is int and 0 <= passed <= total and total > 0:
                    outcome = 'pass' if passed == total else 'fail'
                    message = f'{passed}/{total} comparison replicas passed'
            attributes = {
                f'smp.bounds.{key}': str(value)
                for key, value in (check or {}).items()
                if key != 'name' and value is not None
            }
            add(name, 'bounds', check_name, outcome, message, check, attributes)

        goal = exp.get('optimization_goal')
        if goal is not None or config.get('optimization_goal') is not None:
            result = goal.get('result') if goal else None
            outcome = 'error'
            message = 'Optimization result is missing'
            attributes = {}
            if result is not None:
                failed = result['is_significant_change'] and not result['is_improvement']
                outcome = 'fail' if failed else 'pass'
                message = 'Significant degradation' if failed else 'No significant degradation'
                attributes['smp.statistically_erratic'] = str(result['is_erratic']).lower()
                attributes['smp.percent_change'] = str(result['percent_change'])
                attributes['smp.percent_change_ci_lower'] = str(result['confidence_interval']['lower'])
                attributes['smp.percent_change_ci_upper'] = str(result['confidence_interval']['upper'])
                attributes['smp.trial_count'] = str(goal['trial_count'])
                attributes['smp.max_trial_count'] = str(goal['max_trial_count'])
                if goal.get('goal'):
                    attributes['smp.optimization_goal'] = goal['goal']
                if result['is_erratic']:
                    add(name, 'analysis', 'statistical_stability', 'error', 'Statistically erratic result', result)
            add(name, 'optimization_goal', 'optimization_goal', outcome, message, goal, attributes)

        if not exp:
            add(name, 'execution', 'experiment_result', 'error', 'Selected experiment has no result', config)

    analysis_errors = defaultdict(list)
    for error in report['analysis_errors']:
        analysis_errors[(error['experiment'], error['analysis_type'], error['check_name'])].append(error)
    for (name, kind, check), errors in sorted(analysis_errors.items()):
        add(name, 'analysis', f'{kind}::{check}', 'error', 'Analysis failed', errors)

    failures = defaultdict(list)
    for failure in report['failed_replicates']:
        failures[failure['experiment']].append(failure)
    for name, failed in sorted(failures.items()):
        add(name, 'execution', 'replicate_execution', 'error', 'Replica execution failures occurred', failed)

    identities = [row['tags']['smp.evaluation_id'] for row in evaluations]
    if len(identities) != len(set(identities)):
        raise ValueError('Duplicate evaluation identities')
    suites = []
    for name in sorted({row['experiment'] for row in evaluations}):
        rows = [row for row in evaluations if row['experiment'] == name]
        suites.append({'name': f'smp.health.{name}', 'evaluations': rows, **counts(rows)})
    return {
        'service': SERVICE,
        'upload_enabled': False,
        'evaluations': evaluations,
        'suites': suites,
        **counts(evaluations),
    }


def counts(rows):
    return {
        'tests': len(rows),
        'failures': sum(row['outcome'] == 'fail' for row in rows),
        'errors': sum(row['outcome'] == 'error' for row in rows),
    }


def render_artifacts(smp, report_path, output_dir, config_dir, pr_path, env=None):
    """Render review artifacts with the installed SMP CLI, without network calls."""
    env = os.environ if env is None else env
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)
    xml_path = output / 'experiment-health.junit.xml'
    # A failed rerender must not leave stale results that look current.
    for name in ['experiment-health.junit.xml', 'summary.json', 'upload-plan.json']:
        (output / name).unlink(missing_ok=True)
    command = [
        smp,
        'experiments',
        'resolve',
        '--target-config-dir',
        config_dir,
        '--manifest',
        f'{config_dir}/selection.yaml',
        '--output',
        'json',
    ]
    for variable, flag in [('SMP_LABELS', '--label'), ('SMP_INVOLVED', '--involved-team')]:
        if env.get(variable):
            command.extend([flag, env[variable]])
    selected = json.loads(subprocess.run(command, check=True, capture_output=True, text=True).stdout)
    report = read_json(report_path)
    pr = read_json(pr_path) if Path(pr_path).is_file() else {'status': 'unavailable'}
    context = ci_context(env, pr)
    payload = build_payload(report, context, expected_experiments(config_dir, selected))
    write_json(output / 'selected-experiments.json', selected)
    write_json(output / 'pr-context.json', pr)
    write_json(output / 'selection-inputs.json', {key: env.get(key, '') for key in ['SMP_LABELS', 'SMP_INVOLVED']})
    write_json(output / 'context.json', context)
    write_json(output / 'evaluations.json', payload)
    command = [
        smp,
        'report',
        'render',
        '--report',
        str(report_path),
        '--template-file',
        str(TEMPLATE),
        '--extra',
        str(output / 'evaluations.json'),
    ]
    rendered = subprocess.run(command, check=True, capture_output=True, text=True).stdout
    tree = ET.fromstring(rendered)
    testcases = tree.findall('.//testcase')
    actual = {
        'tests': len(testcases),
        'failures': len(tree.findall('.//testcase/failure')),
        'errors': len(tree.findall('.//testcase/error')),
    }
    if actual != counts(payload['evaluations']):
        raise ValueError('Rendered XML counts differ from source evaluations')
    xml_path.write_text(rendered, encoding='utf-8')
    write_json(output / 'summary.json', summarize([payload]))
    write_json(
        output / 'upload-plan.json',
        {
            'enabled': False,
            'service': SERVICE,
            'path': str(xml_path),
            'report_kind': 'experiment_health',
            'report_version': '1',
        },
    )
    return actual


def summarize(payloads, experiment=None):
    """Return all evaluations and the newest reported run per PR and experiment."""
    unique = {}
    for payload in payloads:
        for row in payload['evaluations']:
            if experiment and row['experiment'] != experiment:
                continue
            key = row['tags']['smp.evaluation_id']
            if key in unique and unique[key] != row:
                raise ValueError(f'Conflicting evaluation: {key}')
            unique[key] = row
    rows = list(unique.values())

    def order(row):
        tags = row['tags']
        return (
            int(tags['smp.parent_pipeline_id']),
            int(tags['smp.ci_job_id']),
            int(tags['smp.analysis_end']),
            tags['smp.job_id'],
        )

    latest = {}
    for row in rows:
        tags = row['tags']
        if tags['smp.context'] == 'pr':
            key = (tags['smp.pr_number'], row['experiment'])
            latest[key] = max(latest.get(key, order(row)), order(row))
    latest_rows = [
        row
        for row in rows
        if row['tags']['smp.context'] == 'pr'
        and order(row) == latest[(row['tags']['smp.pr_number'], row['experiment'])]
    ]

    def aggregate(selected):
        groups = defaultdict(list)
        for row in selected:
            groups[(row['tags']['smp.context'], row['experiment'], row['kind'])].append(row)
        result = []
        for (context, name, kind), evaluations in sorted(groups.items()):
            passed = sum(row['outcome'] == 'pass' for row in evaluations)
            failed = sum(row['outcome'] == 'fail' for row in evaluations)
            result.append(
                {
                    'context': context,
                    'experiment': name,
                    'kind': kind,
                    'pass': passed,
                    'fail': failed,
                    'error': sum(row['outcome'] == 'error' for row in evaluations),
                    'pass_rate': 100 * passed / (passed + failed) if passed + failed else None,
                }
            )
        return {'counts': result, 'evaluations': selected}

    return {'all_evaluations': aggregate(rows), 'latest_per_pr': aggregate(latest_rows)}
