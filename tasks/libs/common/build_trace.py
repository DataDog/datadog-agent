"""
Build-trace fragments: per-CI-job, machine-readable timing spans.

Each instrumented build job appends spans to a fragment at
``$CI_PROJECT_DIR/build-trace/<job-slug>.json``. A workstream-side harvester
later stitches these fragments together with GitLab job timings and the omnibus
``build-summary.json`` into a unified ``deb``/``x86`` build trace
(segment -> component -> duration).

This is additive instrumentation layered on top of ``timed``; it does not
replace ``timed``, ``gitlab_section`` or ``send_build_metrics``. Recording is
best-effort: it must never raise into the build it is measuring.
"""

from __future__ import annotations

import json
import os
import time
from contextlib import contextmanager

SCHEMA_VERSION = "1"


def _trace_dir() -> str:
    base = os.environ.get("CI_PROJECT_DIR") or os.getcwd()
    return os.path.join(base, "build-trace")


def _job_slug() -> str:
    # GitLab provides CI_JOB_NAME_SLUG; fall back to the name, then to "local".
    return os.environ.get("CI_JOB_NAME_SLUG") or os.environ.get("CI_JOB_NAME") or "local"


def _fragment_path() -> str:
    return os.path.join(_trace_dir(), f"{_job_slug()}.json")


def record_span(name: str, duration_s: float, cached: bool | None = None, meta: dict | None = None) -> None:
    """Append one timing span to this job's build-trace fragment.

    Safe to call from successive ``dda inv`` invocations within the same CI job:
    the fragment is read-modify-written so spans accumulate. CI job steps run
    sequentially, so no locking is needed. Never raises -- a tracing failure must
    not break a build.
    """
    try:
        path = _fragment_path()
        os.makedirs(os.path.dirname(path), exist_ok=True)

        fragment = {"schema_version": SCHEMA_VERSION, "job": _job_slug(), "spans": []}
        if os.path.exists(path):
            try:
                with open(path) as f:
                    fragment = json.load(f)
            except (json.JSONDecodeError, OSError):
                # Corrupt/partial fragment: start fresh rather than fail the build.
                fragment = {"schema_version": SCHEMA_VERSION, "job": _job_slug(), "spans": []}

        # Segment is a property of the job, not the span; the harvester maps
        # job -> segment. We record it here only when CI sets it, for readability.
        segment = os.environ.get("BUILD_TRACE_SEGMENT")
        if segment:
            fragment["segment"] = segment
        arch = os.environ.get("PACKAGE_ARCH") or os.environ.get("ARCH")
        if arch:
            fragment["arch"] = arch

        span: dict = {"name": name, "duration_s": round(float(duration_s), 3)}
        if cached is not None:
            span["cached"] = bool(cached)
        if meta:
            span["meta"] = meta
        fragment.setdefault("spans", []).append(span)

        with open(path, "w") as f:
            json.dump(fragment, f, indent=2)
    except Exception as e:
        print(f"build-trace: failed to record span {name!r}: {e}")


@contextmanager
def trace_span(name: str, cached: bool | None = None, meta: dict | None = None):
    """Time a block and record it as a build-trace span (mirrors ``timed``)."""
    start = time.perf_counter()
    try:
        yield
    finally:
        record_span(name, time.perf_counter() - start, cached=cached, meta=meta)
