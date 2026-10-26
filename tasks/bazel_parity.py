"""Compare Bazel and omnibus Linux packages and report migration progress."""

import hashlib
import json
import os
import re
import subprocess
import tempfile
import time
from collections import Counter
from dataclasses import asdict
from pathlib import Path

import yaml
from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.files_inventory import _compare_inventories
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.package.metadata import read_deb_metadata, read_rpm_metadata
from tasks.static_quality_gates.experimental_gates import FileInfo, measure_package_local

PRODUCTS = {'datadog-agent': 'agent'}
ARCHITECTURES = {'amd64': {'deb': 'amd64', 'rpm': 'x86_64'}}
FORMATS = {'deb': read_deb_metadata, 'rpm': read_rpm_metadata}


def find_packages(directory: str, product: str, package_format: str, arch: str) -> Path | None:
    """Require one unambiguous package, excluding debug/FIPS/other products."""
    native_arch = ARCHITECTURES[arch][package_format]
    pattern = (
        rf'{re.escape(product)}_\d[^/]*_{native_arch}\.deb'
        if package_format == 'deb'
        else rf'{re.escape(product)}-\d[^/]*\.{native_arch}\.rpm'
    )
    matches = sorted(p for p in Path(directory).rglob(f'*.{package_format}') if re.fullmatch(pattern, p.name))
    if len(matches) > 1:
        raise ValueError(f'Ambiguous {product}/{package_format}/{arch} packages in {directory}: {matches}')
    return matches[0] if matches else None


def file_tasks(previous: list[FileInfo], current: list[FileInfo]) -> tuple[list[dict], dict]:
    added, removed, changed = _compare_inventories(previous, current, compare_all_sizes=True, compare_symlinks=True)
    groups = {info.relative_path.split('/')[0]: [] for info in previous + current}
    for result, files in (('missing_in_bazel', removed), ('extra_in_bazel', added)):
        for info in files:
            groups[info.relative_path.split('/')[0]].append(
                {'path': info.relative_path, 'result': result, 'file': asdict(info)}
            )
    for path, change in changed.items():
        groups[path.split('/')[0]].append(
            {
                'path': path,
                'result': 'mismatch',
                'flags': [flag.name.lower() for flag in type(change.flags) if flag & change.flags],
                'omnibus': asdict(change.previous),
                'bazel': asdict(change.current),
            }
        )
    tasks = [
        {
            'check': 'file',
            'item': f'directory_{directory}',
            'title': f'Files under /{directory}',
            'differences': sorted(differences, key=lambda d: d['path']),
        }
        for directory, differences in sorted(groups.items())
    ]
    counts = {
        'match': len(previous) - len(removed) - len(changed),
        'missing_in_bazel': len(removed),
        'extra_in_bazel': len(added),
        'mismatch': len(changed),
    }
    return tasks, counts


def build_tags(**values: str) -> list[str]:
    tags = []
    for key, raw in values.items():
        value = re.sub(r'[^a-z0-9_./:-]', '_', str(raw).lower()) or 'unknown'
        # Datadog limits the whole key:value tag to 200 characters. Retain a
        # hash for truncated values so distinct long branches/items stay distinct.
        limit = 200 - len(key) - 1
        if len(value) > limit:
            value = value[: limit - 13] + '_' + hashlib.sha256(str(raw).encode()).hexdigest()[:12]
        tags.append(f'{key}:{value}')
    return tags


def should_submit(dry_run: bool = False) -> bool:
    return not dry_run and (
        os.environ.get('CI_COMMIT_BRANCH') == 'main' or os.environ.get('BAZEL_PARITY_SUBMIT') == 'true'
    )


def metric_values(report: dict) -> list[dict]:
    metrics = []
    for item in report['tasks']:
        metrics.append(
            {
                'name': 'datadog_agent.bazel_parity.task',
                'value': 1,
                'tags': build_tags(
                    **{k: item[k] for k in ('check', 'product', 'format', 'arch', 'status', 'item')},
                    branch=report['branch'],
                ),
            }
        )
    for package in report['packages']:
        for result, count in package.get('files', {}).items():
            metrics.append(
                {
                    'name': 'datadog_agent.bazel_parity.files',
                    'value': count,
                    'tags': build_tags(
                        product=package['product'],
                        format=package['format'],
                        arch=package['arch'],
                        result=result,
                        branch=report['branch'],
                    ),
                }
            )
    return metrics


def submit_metrics(report: dict, dry_run: bool) -> None:
    if not should_submit(dry_run):
        print(f'Dry run: {len(report["metrics"])} metrics recorded in report; nothing submitted.')
        return
    timestamp = int(time.time())
    series = [create_gauge(m['name'], timestamp, m['value'], m['tags']) for m in report['metrics']]
    # Bound request size as the number of checks grows.
    for offset in range(0, len(series), 500):
        send_metrics(series[offset : offset + 500])


def measure_inventory(ctx, package: Path, product: str, package_format: str, arch: str) -> list[FileInfo]:
    with tempfile.TemporaryDirectory(prefix='bazel-parity-') as tmp:
        output = Path(tmp) / 'inventory.yml'
        measure_package_local(
            ctx,
            str(package),
            f'static_quality_gate_{PRODUCTS[product]}_{package_format}_{arch}',
            output_path=str(output),
            build_job_name=os.environ.get('CI_JOB_NAME', 'bazel-parity'),
        )
        with output.open(encoding='utf-8') as f:
            return [FileInfo(**info) for info in yaml.safe_load(f)['file_inventory']]


def build_report(ctx, omnibus_dir: str, bazel_dir: str) -> dict:
    report = {'schema_version': 1, 'branch': os.environ.get('CI_COMMIT_BRANCH', 'local'), 'packages': [], 'tasks': []}
    for product in PRODUCTS:
        for package_format, reader in FORMATS.items():
            for arch in ARCHITECTURES:
                scope = {'product': product, 'format': package_format, 'arch': arch}
                package = {**scope}
                report['packages'].append(package)
                try:
                    old = find_packages(omnibus_dir, product, package_format, arch)
                    new = find_packages(bazel_dir, product, package_format, arch)
                    package.update(omnibus=str(old) if old else None, bazel=str(new) if new else None)
                    if old is None or new is None:
                        raise ValueError(f'Missing {"omnibus" if old is None else "Bazel"} package')
                    previous = measure_inventory(ctx, old, product, package_format, arch)
                    current = measure_inventory(ctx, new, product, package_format, arch)
                    files, package['files'] = file_tasks(previous, current)
                    report['tasks'].extend({**scope, **item} for item in files)
                    previous_metadata, current_metadata = reader(old), reader(new)
                    package['metadata'] = {'omnibus': previous_metadata, 'bazel': current_metadata}
                    for field in sorted(previous_metadata.keys() | current_metadata.keys()):
                        old_value, new_value = previous_metadata.get(field, ''), current_metadata.get(field, '')
                        report['tasks'].append(
                            {
                                **scope,
                                'check': 'metadata',
                                'item': f'field_{field}',
                                'title': f'Package metadata: {field}',
                                'differences': [{'omnibus': old_value, 'bazel': new_value}]
                                if old_value != new_value
                                else [],
                            }
                        )
                except (OSError, ValueError, RuntimeError, subprocess.SubprocessError, UnexpectedExit) as error:
                    package['error'] = str(error)
                    report['tasks'].append(
                        {
                            **scope,
                            'check': 'metadata',
                            'item': 'package_comparison',
                            'title': 'Package comparison must complete',
                            'differences': [{'error': str(error)}],
                        }
                    )
    for item in report['tasks']:
        item['status'] = 'remaining' if item['differences'] else 'done'
    report['summary'] = {'done': 0, 'remaining': 0}
    report['summary'].update(Counter(item['status'] for item in report['tasks']))
    report['metrics'] = metric_values(report)
    return report


def write_report(report: dict, output: str) -> str:
    path = Path(output)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2) + '\n', encoding='utf-8')
    summary = report['summary']
    complete = summary['done']
    total = complete + summary['remaining']
    lines = [
        '# Bazel Linux package parity',
        '',
        f'{complete}/{total} tasks complete; {summary["remaining"]} remaining.',
        '',
        '| Format | Check | Item | Status |',
        '| --- | --- | --- | --- |',
    ]
    for item in report['tasks']:
        lines.append(f'| {item["format"]} | {item["check"]} | {item["item"]} | {item["status"]} |')
    lines.extend(
        [
            '',
            '## File counts',
            '',
            '| Format | Match | Missing in Bazel | Extra in Bazel | Mismatch |',
            '| --- | --- | --- | --- | --- |',
        ]
    )
    for package in report['packages']:
        if 'files' in package:
            counts = package['files']
            lines.append(
                f'| {package["format"]} | {counts["match"]} | {counts["missing_in_bazel"]} | '
                f'{counts["extra_in_bazel"]} | {counts["mismatch"]} |'
            )
        if 'error' in package:
            lines.extend(['', f'{package["format"]} comparison failed: {package["error"]}'])
    markdown = '\n'.join(lines) + '\n'
    path.with_suffix('.md').write_text(markdown, encoding='utf-8')
    return markdown


@task(
    help={
        'omnibus_dir': 'Directory containing omnibus DEB/RPM artifacts',
        'bazel_dir': 'Directory containing Bazel DEB/RPM artifacts',
        'dry_run': 'Write reports without submitting metrics, even on main',
    }
)
def check(
    ctx,
    omnibus_dir,
    bazel_dir,
    output='bazel_parity_report.json',
    dry_run=False,
):
    """Report package parity; remaining tasks fail this advisory CI check."""
    report = build_report(ctx, omnibus_dir, bazel_dir)
    print(write_report(report, output))
    submit_metrics(report, dry_run)
    if report['summary']['remaining']:
        raise Exit(f'{report["summary"]["remaining"]} Bazel parity tasks remaining; see {output}', code=1)
