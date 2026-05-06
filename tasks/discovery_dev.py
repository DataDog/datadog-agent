"""
Tasks for building the discovery-dev image used by the krakend discovery
e2e test in integrations-core.
"""

from __future__ import annotations

import os
import re
import sys

from invoke.tasks import task

DOCKERFILE = "test/dockerfiles/discovery-dev/Dockerfile"
DEFAULT_TAG = "datadog/agent-dev:discovery-local"

LIBPYTHON_RE = re.compile(rb"libpython3\.(\d+)\.so\.1\.0")


def _libpython_version(so_path: str) -> str | None:
    """Return the python X.Y version the .so links against, or None if undetectable."""
    with open(so_path, "rb") as f:
        m = LIBPYTHON_RE.search(f.read())
    return f"3.{m.group(1).decode()}" if m else None


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

    # `dda inv agent.build` re-links rtloader against the host's
    # python3.X-dev, silently overwriting the bazel-built .so files in
    # dev/lib/. Detect by checking that the libpython the rtloader points
    # at exists in dev/embedded/lib/ (where bazel installs its python).
    py_version = _libpython_version(rtloader_so)
    if py_version is None:
        sys.exit(f"unable to detect libpython version in {rtloader_so}")
    embedded_libpython = os.path.join(repo_path, f"dev/embedded/lib/libpython{py_version}.so.1.0")
    if not os.path.isfile(embedded_libpython):
        sys.exit(
            f"{rtloader_so} links against libpython{py_version}, but "
            f"{embedded_libpython} is missing. This usually means a recent "
            f"`dda inv agent.build` overwrote the bazel-built rtloader. Run:\n"
            f"  dda inv rtloader.install-with-bazel\n"
            f"  cp -P dev/embedded/lib/libdatadog-agent-rtloader* dev/lib/\n"
            f"  cp -P dev/embedded/lib/libdatadog-agent-three.so dev/lib/\n"
            f"See docs/superpowers/2026-05-06-discover-e2e-smoke.md."
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
