"""
Tasks for building the discovery-dev image used by the krakend discovery
e2e test in integrations-core.
"""

from __future__ import annotations

import os
import sys

from invoke.tasks import task

DOCKERFILE = "test/dockerfiles/discovery-dev/Dockerfile"
DEFAULT_TAG = "datadog/agent-dev:discovery-local"


@task(
    help={
        "tag": f"Image tag to produce (default: {DEFAULT_TAG}).",
        "base_image": "Override BASE_IMAGE build-arg (default: nightly-main-py3-jmx).",
    }
)
def build_image(ctx, tag: str = DEFAULT_TAG, base_image: str | None = None):
    """Build the local discovery-dev agent image."""
    repo_path = os.getcwd()

    agent_bin = os.path.join(repo_path, "bin/agent/agent")
    rtloader_so = os.path.join(repo_path, "dev/lib/libdatadog-agent-three.so")
    if not os.path.isfile(agent_bin) or not os.path.isfile(rtloader_so):
        sys.exit(
            "missing artifacts; run `dda inv agent.build` and "
            "`dda inv rtloader.install-with-bazel` first, then copy "
            "the bazel-built .so files into dev/lib (see "
            "docs/superpowers/2026-05-06-discover-e2e-smoke.md)."
        )

    cmd = [
        "docker",
        "build",
        "-f",
        DOCKERFILE,
        "--build-arg",
        f"REPO_PATH={repo_path}",
    ]
    if base_image:
        cmd += ["--build-arg", f"BASE_IMAGE={base_image}"]
    cmd += ["-t", tag, "."]

    ctx.run(" ".join(cmd))
