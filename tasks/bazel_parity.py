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
from typing import Any

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.files_inventory import _compare_inventories
from tasks.libs.common.datadog_api import create_gauge, send_event, send_metrics
from tasks.libs.package.binaries import binary_signatures, signature_differences
from tasks.libs.package.metadata import read_deb_metadata, read_rpm_metadata
from tasks.libs.package.size import CENTOS_OS, DEBIAN_OS, extract_package
from tasks.static_quality_gates.experimental_gates import FileInfo, FileUtilities

PRODUCTS = {'datadog-agent': 'agent'}
ARCHITECTURES = {'amd64': {'deb': 'amd64', 'rpm': 'x86_64'}}
FORMATS = {'deb': read_deb_metadata, 'rpm': read_rpm_metadata}
LOG_PREFIX = 'BAZEL_PARITY'
# Derived from file sizes, which legitimately differ between builds: only require a value.
PRESENCE_ONLY_FIELDS = {'installed-size'}
PACKAGE_OS = {'deb': DEBIAN_OS, 'rpm': CENTOS_OS}


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


def file_tasks(
    previous: list[FileInfo],
    current: list[FileInfo],
    previous_binaries: dict[str, dict] | None = None,
    current_binaries: dict[str, dict] | None = None,
) -> tuple[list[dict], dict]:
    previous_binaries, current_binaries = previous_binaries or {}, current_binaries or {}
    added, removed, changed = _compare_inventories(previous, current)
    groups = {info.relative_path.split('/')[0]: [] for info in previous + current}
    for result, files in (('missing_in_bazel', removed), ('extra_in_bazel', added)):
        for info in files:
            groups[info.relative_path.split('/')[0]].append(
                {'path': info.relative_path, 'result': result, 'file': asdict(info)}
            )
    current_files = {info.relative_path: info for info in current}
    mismatch_count = 0
    for old in previous:
        path = old.relative_path
        new = current_files.get(path)
        if new is None:
            continue
        change = changed.get(path)
        flags = [flag.name.lower() for flag in type(change.flags) if flag & change.flags] if change else []
        if bool(old.is_symlink) != bool(new.is_symlink) or old.symlink_target != new.symlink_target:
            flags.append('symlink')
        binary = []
        if path in previous_binaries and path in current_binaries:
            binary = signature_differences(previous_binaries[path], current_binaries[path])
        elif (path in previous_binaries) != (path in current_binaries):
            binary = ['not_elf']
        if binary:
            flags.append('binary')
        if not flags:
            continue
        mismatch_count += 1
        groups[path.split('/')[0]].append(
            {
                'path': path,
                'result': 'mismatch',
                'flags': flags,
                'omnibus': asdict(old),
                'bazel': asdict(new),
                **(
                    {
                        'binary': binary,
                        'omnibus_binary': previous_binaries.get(path),
                        'bazel_binary': current_binaries.get(path),
                    }
                    if binary
                    else {}
                ),
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
        'match': len(previous) - len(removed) - mismatch_count,
        'missing_in_bazel': len(removed),
        'extra_in_bazel': len(added),
        'mismatch': mismatch_count,
    }
    return tasks, counts


def metric_tags(**values: str) -> list[str]:
    tags = []
    for key, raw in values.items():
        value = re.sub(r'[^a-z0-9_./:-]', '_', str(raw).lower()) or 'unknown'
        # Datadog limits the whole key:value tag to 200 characters. Retain a
        # hash for truncated values so distinct long paths stay distinct.
        limit = 200 - len(key) - 1
        if len(value) > limit:
            value = value[: limit - 13] + '_' + hashlib.sha256(str(raw).encode()).hexdigest()[:12]
        tags.append(f'{key}:{value}')
    return tags


def metric_values(report: dict) -> list[dict]:
    metrics = []
    for package in report['packages']:
        scope = {'product': package['product'], 'format': package['format'], 'arch': package['arch']}
        metrics.append(
            {
                'name': 'datadog_agent.bazel_parity.last_run',
                'value': int(time.time()),
                'tags': metric_tags(**scope),
            }
        )
        metrics.append(
            {
                'name': 'datadog_agent.bazel_parity.comparison_failed',
                'value': int('error' in package),
                'tags': metric_tags(**scope),
            }
        )
        for result, count in package.get('files', {}).items():
            metrics.append(
                {
                    'name': 'datadog_agent.bazel_parity.files',
                    'value': count,
                    'tags': metric_tags(**scope, result=result),
                }
            )
    reasons = Counter(
        (item['product'], item['format'], item['arch'], reason)
        for item in report['tasks']
        for difference in item['differences']
        for reason in difference.get('binary', [])
    )
    for (product, package_format, arch, reason), count in sorted(reasons.items()):
        metrics.append(
            {
                'name': 'datadog_agent.bazel_parity.binary_difference',
                'value': count,
                'tags': metric_tags(product=product, format=package_format, arch=arch, reason=reason),
            }
        )
    return metrics


def run_event() -> dict:
    branch = os.environ.get('CI_COMMIT_REF_NAME', 'unknown')
    sha = os.environ.get('CI_COMMIT_SHA', 'unknown')
    return {
        'title': f'Bazel parity: {branch}@{sha[:12]}',
        'text': (
            '%%%\n'
            f'Commit: [{sha[:12]}](https://github.com/DataDog/datadog-agent/commit/{sha})\n\n'
            f'Pipeline: {os.environ.get("CI_PIPELINE_URL", "unknown")}\n'
            '%%%'
        ),
        'tags': ['bazel_parity'] + metric_tags(**{'git.branch': branch, 'git.commit.sha': sha}),
    }


def submit_metrics(report: dict) -> None:
    if os.environ.get('CI_COMMIT_BRANCH') != 'main' and os.environ.get('BAZEL_PARITY_SUBMIT') != 'true':
        print(f'Not submitting {len(report["metrics"])} metrics.')
        return
    timestamp = int(time.time())
    branch = metric_tags(**{'git.branch': os.environ.get('CI_COMMIT_REF_NAME', 'unknown')})
    series = [create_gauge(m['name'], timestamp, m['value'], m['tags'] + branch) for m in report['metrics']]
    # Bound request size as the number of checks grows.
    for offset in range(0, len(series), 500):
        send_metrics(series[offset : offset + 500])
    send_event(**run_event())


def measure_inventory(ctx, package: Path, package_format: str, readelf: str) -> tuple[list[FileInfo], dict[str, dict]]:
    with tempfile.TemporaryDirectory(prefix='bazel-parity-') as tmp:
        extract_package(ctx, PACKAGE_OS[package_format], str(package), tmp)
        inventory = FileUtilities.walk_files(tmp, False, include_directories=True)
        return inventory, binary_signatures(tmp, [info.relative_path for info in inventory], readelf)


def signature_state(package: Path) -> str:
    """`dd-pkg sign` writes the detached signature of a package next to it, as <package>.SHA256.sig."""
    signature = package.with_name(package.name + '.SHA256.sig')
    return 'signed' if signature.is_file() and signature.stat().st_size else 'unsigned'


def metadata_differs(field: str, old_value: str, new_value: str) -> bool:
    if field in PRESENCE_ONLY_FIELDS:
        return (old_value not in ('', '0')) != (new_value not in ('', '0'))
    return old_value != new_value


def build_report(ctx, omnibus_dir: str, bazel_dir: str, readelf: str) -> dict:
    report: dict[str, Any] = {'packages': [], 'tasks': []}
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
                    previous, previous_binaries = measure_inventory(ctx, old, package_format, readelf)
                    current, current_binaries = measure_inventory(ctx, new, package_format, readelf)
                    files, package['files'] = file_tasks(previous, current, previous_binaries, current_binaries)
                    report['tasks'].extend({**scope, **item} for item in files)
                    previous_metadata = {**reader(old), 'signature': signature_state(old)}
                    current_metadata = {**reader(new), 'signature': signature_state(new)}
                    for field in sorted(previous_metadata.keys() | current_metadata.keys()):
                        old_value, new_value = previous_metadata.get(field, ''), current_metadata.get(field, '')
                        report['tasks'].append(
                            {
                                **scope,
                                'check': 'metadata',
                                'item': f'field_{field}',
                                'title': f'Package metadata: {field}',
                                'differences': [{'omnibus': old_value, 'bazel': new_value}]
                                if metadata_differs(field, old_value, new_value)
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


def format_difference(item: dict, difference: dict) -> str:
    """One line per difference: job logs are indexed line by line, so each must stand alone."""
    if item['check'] == 'file':
        line = f'file {difference["result"]} {difference["path"]}'
        if 'flags' in difference:
            line += f' flags={",".join(difference["flags"])}'
        if 'binary' in difference:
            line += f' binary={",".join(difference["binary"])}'
        for side in ('omnibus', 'bazel'):
            error = (difference.get(f'{side}_binary') or {}).get('unreadable')
            if error:
                line += f' {side}_error={json.dumps(error)}'
        return line
    if 'error' in difference:
        return f'error {json.dumps(difference["error"])}'
    field = item['item'].removeprefix('field_')
    omnibus, bazel = difference['omnibus'], difference['bazel']
    if '\n' not in omnibus and '\n' not in bazel:
        return f'metadata {field} omnibus={json.dumps(omnibus)} bazel={json.dumps(bazel)}'
    # Multi-line values (dependency lists, scripts) are only readable as the lines that differ.
    omnibus_only = list((Counter(omnibus.splitlines()) - Counter(bazel.splitlines())).elements())
    bazel_only = list((Counter(bazel.splitlines()) - Counter(omnibus.splitlines())).elements())
    if not omnibus_only and not bazel_only:
        return f'metadata {field} same lines in a different order'
    return f'metadata {field} omnibus_only={json.dumps(omnibus_only)} bazel_only={json.dumps(bazel_only)}'


def print_report(report: dict) -> None:
    summary = report['summary']
    total = summary['done'] + summary['remaining']
    print(f'{LOG_PREFIX} summary {summary["done"]}/{total} tasks complete; {summary["remaining"]} remaining')
    for package in report['packages']:
        scope = f'{package["product"]}/{package["format"]}/{package["arch"]}'
        if 'files' in package:
            counts = ' '.join(f'{result}={count}' for result, count in package['files'].items())
            print(f'{LOG_PREFIX} {scope} counts {counts}')
    for item in report['tasks']:
        scope = f'{item["product"]}/{item["format"]}/{item["arch"]}'
        for difference in item['differences']:
            print(f'{LOG_PREFIX} {scope} {format_difference(item, difference)}')


@task(
    help={
        'omnibus_dir': 'Directory containing omnibus DEB/RPM artifacts',
        'bazel_dir': 'Directory containing Bazel DEB/RPM artifacts',
        'readelf': 'Path to the readelf binary built from //bazel/tools:readelf',
    }
)
def check(ctx, omnibus_dir, bazel_dir, readelf):
    """Report package parity; remaining tasks fail this advisory CI check."""
    report = build_report(ctx, omnibus_dir, bazel_dir, readelf)
    print_report(report)
    submit_metrics(report)
    if report['summary']['remaining']:
        raise Exit(f'{report["summary"]["remaining"]} Bazel parity tasks remaining', code=1)
