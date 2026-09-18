#!/usr/bin/env python3
"""Install a pinned uv in the host's shared cache and print its executable path."""

import argparse
import fcntl
import os
import platform
import re
import signal
import subprocess
import sys
import tempfile
from pathlib import Path

DOWNLOAD_TIMEOUT_SECONDS = 300


def is_executable(path):
    return path.is_file() and os.access(path, os.X_OK)


def target_triple():
    system, machine = platform.system(), platform.machine()
    architecture = "aarch64" if machine in ("arm64", "aarch64") else "x86_64"
    if system == "Darwin":
        return f"{architecture}-apple-darwin"
    if system == "Linux":
        return f"{architecture}-unknown-linux-gnu"
    raise ValueError(f"unsupported platform: {system}-{machine}")


def install(version, root):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError(f"invalid uv version {version!r}; expected a release version such as 0.8.4")

    install_dir = root / version
    binary = install_dir / "uv"
    if is_executable(binary):
        return binary

    install_dir.mkdir(parents=True, exist_ok=True)
    # Keep the lock file in place so every waiter locks the same inode.
    # The kernel releases the lock when its descriptor closes, including when the installer is killed.
    with (install_dir / ".install.lock").open("a") as lock_file:
        fcntl.flock(lock_file, fcntl.LOCK_EX)
        if is_executable(binary):
            return binary

        triple = target_triple()
        print(f"install-uv: downloading uv {version} for {triple}", file=sys.stderr)
        # Staging beside the destination makes publication atomic, so waiters never see a partial
        # executable. Child output goes to stderr because this script's stdout is the path it returns.
        with tempfile.TemporaryDirectory(prefix=".staging-", dir=install_dir) as directory:
            staging = Path(directory)
            archive = staging / "uv.tar.gz"
            subprocess.run(
                [
                    "curl",
                    "-fsSL",
                    "--retry",
                    "4",
                    "-o",
                    str(archive),
                    f"https://github.com/astral-sh/uv/releases/download/{version}/uv-{triple}.tar.gz",
                ],
                check=True,
                stdout=sys.stderr,
                # Bound all retries together so a stalled download releases the lock for other jobs.
                timeout=DOWNLOAD_TIMEOUT_SECONDS,
            )
            subprocess.run(
                ["tar", "-xzf", str(archive), "-C", str(staging), "--strip-components", "1"],
                check=True,
                stdout=sys.stderr,
            )
            executable = staging / "uv"
            if not is_executable(executable):
                raise OSError(f"uv {version} is missing from {executable} or is not executable")
            os.replace(executable, binary)

    return binary


def terminate(signum, _frame):
    # Raising on SIGTERM unwinds the contexts to stop the child and remove staging files on cancellation.
    raise SystemExit(128 + signum)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    args = parser.parse_args()
    root = Path(os.environ.get("UV_INSTALL_ROOT") or Path.home() / ".cache/datadog-agent-ci/uv").resolve()
    signal.signal(signal.SIGTERM, terminate)
    print(install(args.version, root))


if __name__ == "__main__":
    main()
