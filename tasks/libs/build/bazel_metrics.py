"""Parse Bazel BEP and profile files collected during CI jobs and emit Datadog metrics."""

from __future__ import annotations

import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


def _parse_bep_timestamp(ts_str: str) -> float | None:
    """Convert a BEP RFC3339 timestamp (with possible nanoseconds) to a Unix timestamp in ms."""
    if not ts_str:
        return None
    # datetime.fromisoformat handles up to microseconds; strip excess sub-second digits
    ts_str = re.sub(r'(\.\d{6})\d*Z$', r'\1+00:00', ts_str)
    if ts_str.endswith('Z'):
        ts_str = ts_str[:-1] + '+00:00'
    try:
        return datetime.fromisoformat(ts_str).timestamp() * 1000
    except ValueError:
        return None


def _parse_bep_file(path: Path) -> dict:
    """Parse a single BEP NDJSON file and return aggregate build stats."""
    total_actions = 0
    remote_cache_hits = 0
    start_time_ms: float | None = None
    finish_time_ms: float | None = None

    try:
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    event = json.loads(line)
                except json.JSONDecodeError:
                    continue

                if 'started' in event:
                    start_time_ms = _parse_bep_timestamp(event['started'].get('startTime', ''))
                elif 'finished' in event:
                    finish_time_ms = _parse_bep_timestamp(event['finished'].get('finishTime', ''))
                elif 'action' in event:
                    action = event['action']
                    if action.get('success', False):
                        total_actions += 1
                        if action.get('cachedRemotely', False):
                            remote_cache_hits += 1
    except OSError as e:
        print(f"Warning: could not read BEP file {path}: {e}", file=sys.stderr)
        return {}

    duration_s = None
    if start_time_ms is not None and finish_time_ms is not None:
        duration_s = (finish_time_ms - start_time_ms) / 1000.0

    return {
        'total_actions': total_actions,
        'remote_cache_hits': remote_cache_hits,
        'duration_s': duration_s,
    }


def _parse_profile_file(path: Path) -> dict:
    """Parse a Bazel profile JSON (Chrome tracing format) and return file-change scan duration.

    The file-change scan duration is approximated as the elapsed time from build start
    (ts=0 in the profile) to when the Loading phase begins. This phase is dominated by
    Bazel walking the workspace to detect changed files, and grows as output files accumulate.
    """
    try:
        with open(path) as f:
            data = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        print(f"Warning: could not read profile file {path}: {e}", file=sys.stderr)
        return {}

    trace_events = data.get('traceEvents', [])

    # The loading phase starts after file-change scanning completes.
    # In Bazel profiles it appears as complete events (ph='X') in the 'build phase' category,
    # or with names containing 'Loading'. Take the minimum ts among those events.
    loading_start_us: float | None = None
    for event in trace_events:
        if event.get('ph') != 'X':
            continue
        ts = event.get('ts')
        if not ts:
            continue
        name = event.get('name', '')
        cat = event.get('cat', '')
        if cat == 'build phase' or 'Loading' in name or 'LOADING' in name:
            if loading_start_us is None or ts < loading_start_us:
                loading_start_us = ts

    if loading_start_us is not None:
        return {'file_change_scan_duration_s': loading_start_us / 1_000_000.0}

    return {}


def collect_bazel_metrics(metrics_dir: str | Path, tags: list[str]) -> None:
    """Aggregate BEP and profile files in metrics_dir and emit one data point per metric to Datadog.

    Aggregates across all invocations in the job before emitting, so dashboard queries need
    no extra formulas — one point per job per metric.
    """
    from tasks.libs.common.datadog_api import create_count, create_gauge, send_metrics

    metrics_dir = Path(metrics_dir)
    bep_files = sorted(metrics_dir.glob('bep-*.json'))
    profile_files = sorted(metrics_dir.glob('profile-*.json'))

    if not bep_files and not profile_files:
        print(f"No Bazel metrics files found in {metrics_dir}, skipping.", file=sys.stderr)
        return

    total_actions = 0
    total_remote_cache_hits = 0
    total_build_duration_s = 0.0
    total_file_change_scan_s = 0.0
    invocation_count = len(bep_files)

    for bep_file in bep_files:
        stats = _parse_bep_file(bep_file)
        total_actions += stats.get('total_actions', 0)
        total_remote_cache_hits += stats.get('remote_cache_hits', 0)
        if stats.get('duration_s') is not None:
            total_build_duration_s += stats['duration_s']

    for profile_file in profile_files:
        stats = _parse_profile_file(profile_file)
        if stats.get('file_change_scan_duration_s') is not None:
            total_file_change_scan_s += stats['file_change_scan_duration_s']

    timestamp = int(datetime.now(tz=timezone.utc).timestamp())
    series = []

    if invocation_count > 0:
        series.append(create_count('datadog.ci.bazel.invocation_count', timestamp, invocation_count, tags))
    if total_actions > 0:
        series.append(create_count('datadog.ci.bazel.total_actions', timestamp, total_actions, tags))
        series.append(
            create_gauge(
                'datadog.ci.bazel.remote_cache_hit_rate',
                timestamp,
                total_remote_cache_hits / total_actions,
                tags,
            )
        )
    if total_build_duration_s > 0:
        series.append(create_gauge('datadog.ci.bazel.build_duration_s', timestamp, total_build_duration_s, tags))
    if total_file_change_scan_s > 0:
        series.append(
            create_gauge(
                'datadog.ci.bazel.file_change_scan_duration_s',
                timestamp,
                total_file_change_scan_s,
                tags,
            )
        )

    if not series:
        print("No Bazel metrics to send (all invocations may have been empty builds).", file=sys.stderr)
        return

    print(f"Sending {len(series)} Bazel metric(s) to Datadog. Tags: {tags}")
    send_metrics(series)

    for f in bep_files + profile_files:
        f.unlink(missing_ok=True)
