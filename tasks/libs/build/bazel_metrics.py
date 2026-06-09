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
                elif 'buildMetrics' in event:
                    # buildMetrics is always present; per-action events require
                    # --build_event_publish_all_actions which we don't set.
                    action_summary = event['buildMetrics'].get('actionSummary', {})
                    executed = int(action_summary.get('actionsExecuted', 0))
                    hits = int(action_summary.get('remoteCacheHits', 0))
                    total_actions = executed + hits
                    remote_cache_hits = hits
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
    """Parse a Bazel profile JSON (Chrome tracing format) and return phase durations.

    Returns:
        file_change_scan_duration_s: elapsed time from build start to loading phase start.
            Dominated by Bazel walking the workspace for changed files; grows with output
            file accumulation.
        analysis_phase_duration_s: duration of the analysis phase (reading BUILD files,
            evaluating Starlark, constructing the action graph).
    """
    try:
        with open(path) as f:
            data = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        print(f"Warning: could not read profile file {path}: {e}", file=sys.stderr)
        return {}

    trace_events = data.get('traceEvents', [])

    result = {}
    loading_start_us: float | None = None

    for event in trace_events:
        if event.get('ph') != 'X':
            continue
        ts = event.get('ts')
        if ts is None:
            continue
        name = event.get('name', '')
        cat = event.get('cat', '')

        if cat == 'build phase' or 'Loading' in name or 'LOADING' in name:
            if loading_start_us is None or ts < loading_start_us:
                loading_start_us = ts

        if cat == 'build phase' and ('Analysis' in name or 'ANALYSIS' in name):
            dur = event.get('dur')
            if dur:
                result['analysis_phase_duration_s'] = dur / 1_000_000.0

    if loading_start_us is not None:
        result['file_change_scan_duration_s'] = loading_start_us / 1_000_000.0

    return result


def collect_bazel_metrics(metrics_dir: str | Path, tags: list[str], *, collect_invocations: bool = False) -> None:
    """Aggregate BEP and profile files in metrics_dir and emit one data point per metric to Datadog.

    Aggregates across all invocations in the job before emitting, so dashboard queries need
    no extra formulas — one point per job per metric.

    collect_invocations: emit invocation_count (only meaningful for build jobs, not test/lint).
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
    total_analysis_phase_s = 0.0
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
        if stats.get('analysis_phase_duration_s') is not None:
            total_analysis_phase_s += stats['analysis_phase_duration_s']

    timestamp = int(datetime.now(tz=timezone.utc).timestamp())
    series = []

    if collect_invocations and invocation_count > 0:
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
    if total_analysis_phase_s > 0:
        series.append(
            create_gauge(
                'datadog.ci.bazel.analysis_phase_duration_s',
                timestamp,
                total_analysis_phase_s,
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
