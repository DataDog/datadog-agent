"""Shared UTOF metadata generation from the current CI/environment context."""

from __future__ import annotations

import os
import platform
from datetime import datetime, timezone

from tasks.libs.testing.utof.models import (
    UTOFCIMetadata,
    UTOFEnvironmentMetadata,
    UTOFGitMetadata,
    UTOFMetadata,
)


def generate_metadata(test_system: str, flavor: str = "") -> UTOFMetadata:
    """Generate UTOF metadata from the current environment.

    Args:
        test_system: Identifies the test system (e.g. "unit", "e2e", "kmt", "smp").
        flavor: Optional agent flavor string.
    """
    git = UTOFGitMetadata(
        branch=os.environ.get("CI_COMMIT_REF_NAME", ""),
        commit_sha=os.environ.get("CI_COMMIT_SHA", ""),
        commit_author=os.environ.get("CI_COMMIT_AUTHOR", ""),
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
