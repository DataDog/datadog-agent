"""Builds nosys.so, the LD_PRELOAD shim installed in the agent, cluster-agent, and base-image containers."""

from __future__ import annotations

import os
import shutil

from tasks.libs.build.bazel import bazel

NOSYS_SECCOMP_TARGET = "//Dockerfiles/nosys-seccomp:nosys.so"
NOSYS_SECCOMP_BAZEL_PLATFORMS = {
    "amd64": "//bazel/platforms:linux_x86_64",
    "arm64": "//bazel/platforms:linux_arm64",
}


def build_nosys_so(arch: str, dest_dir: str) -> str:
    """Build nosys.so for `arch` with Bazel, copy it into `dest_dir`, and return the copied file's path."""
    platform_flag = f"--platforms={NOSYS_SECCOMP_BAZEL_PLATFORMS[arch]}"
    bazel("build", platform_flag, NOSYS_SECCOMP_TARGET)
    bazel_bin = bazel("info", "bazel-bin", platform_flag, capture_output=True).strip()
    dest = os.path.join(dest_dir, "nosys.so")
    shutil.copy2(os.path.join(bazel_bin, "Dockerfiles", "nosys-seccomp", "nosys.so"), dest)
    return dest
