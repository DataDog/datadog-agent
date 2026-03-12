"""Shared UTOF metadata generation from the current CI/environment context."""

from __future__ import annotations

import os
import platform
from datetime import datetime, timezone

from invoke import Context

from tasks.libs.testing.utof.models import (
    UTOFCIMetadata,
    UTOFEnvironmentMetadata,
    UTOFGitMetadata,
    UTOFMetadata,
)


def _git_value(ctx: Context, cmd: str) -> str:
    """Run a git command via invoke and return its stripped stdout, or "" on failure."""
    result = ctx.run(cmd, hide=True, warn=True)
    if result and result.ok:
        return result.stdout.strip()
    return ""


def generate_metadata(ctx: Context, test_system: str, flavor: str = "") -> UTOFMetadata:
    """Generate UTOF metadata from the current environment.

    Args:
        ctx: Invoke context for running git commands locally.
        test_system: Identifies the test system (e.g. "unit", "e2e", "kmt", "smp").
        flavor: Optional agent flavor string.
    """
    git = UTOFGitMetadata(
        branch=os.environ.get("CI_COMMIT_REF_NAME") or _git_value(ctx, "git rev-parse --abbrev-ref HEAD"),
        commit_sha=os.environ.get("CI_COMMIT_SHA") or _git_value(ctx, "git rev-parse HEAD"),
        commit_author=os.environ.get("CI_COMMIT_AUTHOR") or _git_value(ctx, "git log -1 --format='%aN <%aE>'"),
    )
    ci = UTOFCIMetadata(
        pipeline_id=os.environ.get("CI_PIPELINE_ID", ""),
        job_id=os.environ.get("CI_JOB_ID", ""),
        job_name=os.environ.get("CI_JOB_NAME", ""),
        job_url=os.environ.get("CI_JOB_URL", ""),
    )
    env = UTOFEnvironmentMetadata(
        os=platform.system(),
        os_version=platform.release(),
        arch=platform.machine(),
        kernel=platform.version() if platform.system() == "Linux" else "",
        agent_flavor=flavor,
    )

    return UTOFMetadata(
        test_system=test_system,
        timestamp=datetime.now(timezone.utc).isoformat(),
        git=git,
        ci=ci,
        environment=env,
    )
