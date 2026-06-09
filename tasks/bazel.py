"""Bazel-related invoke tasks."""

from __future__ import annotations

import os
import sys

from invoke import task


@task
def collect_build_metrics(_ctx):
    """Parse BEP and profile files from .bazel-metrics/ and emit build metrics to Datadog.

    Reads all bep-*.json and profile-*.json files written by the tools/bazel wrapper during
    CI, aggregates across invocations, and emits one data point per metric per CI job.
    """
    from tasks.libs.build.bazel_metrics import collect_bazel_metrics

    project_dir = os.environ.get('CI_PROJECT_DIR')
    if not project_dir:
        print("CI_PROJECT_DIR not set — skipping Bazel metric collection.", file=sys.stderr)
        return

    job_name_slug = os.environ.get('CI_JOB_NAME_SLUG', '')
    branch = os.environ.get('CI_COMMIT_REF_NAME', '')
    pipeline_id = os.environ.get('CI_PIPELINE_ID', '')

    if not job_name_slug or not branch:
        print("Missing CI environment variables — skipping Bazel metric collection.", file=sys.stderr)
        return

    # Derive platform and arch from the job name slug.
    # Pure Bazel jobs: 'bazel-test-linux-amd64', 'bazel-test-macos-arm64', etc.
    # Omnibus-transition jobs: 'datadog-agent-7-x64', 'datadog-agent-7-arm64', etc.
    # Normalize arch aliases and detect platform from known tokens in the slug.
    _ARCH_ALIASES = {'x64': 'amd64', 'aarch64': 'arm64', 'x86-64': 'amd64'}
    parts = job_name_slug.split('-')
    raw_arch = parts[-1] if parts else 'unknown'
    arch = _ARCH_ALIASES.get(raw_arch, raw_arch)
    slug_lower = job_name_slug.lower()
    if 'macos' in slug_lower or 'darwin' in slug_lower:
        platform = 'macos'
    elif 'windows' in slug_lower or 'win' in slug_lower:
        platform = 'windows'
    else:
        platform = 'linux'

    tags = [
        f'job:{job_name_slug}',
        f'branch:{branch}',
        f'pipeline:{pipeline_id}',
        f'platform:{platform}',
        f'arch:{arch}',
        'repository:datadog-agent',
    ]

    metrics_dir = os.path.join(project_dir, '.bazel-metrics')
    collect_invocations = os.environ.get('BAZEL_COLLECT_INVOCATIONS') == 'true'
    collect_bazel_metrics(metrics_dir, tags, collect_invocations=collect_invocations)
