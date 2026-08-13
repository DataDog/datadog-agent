"""Run every `go` command of an invoke task with the Bazel-managed Go SDK.

`deps/go.MODULE.bazel` derives that SDK from `go.work`, so it always matches the
version the repository expects — no host Go installation, and no drift between
what `dda inv` builds and what Bazel builds.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path

from invoke.context import Context, MockContext
from invoke.runners import Runner

from tasks.libs.build.bazel import bazel, output_base
from tasks.libs.common.color import color_message

# A plain file at the root of the SDK repository; its parent directory is GOROOT.
GO_SDK_ROOT_LABEL = "@go_work_sdk//:ROOT"

# `go build …`, `go.exe test …`, `GOOS=windows go build …`, `cmd && go run …`, but
# neither `gofmt` nor `golangci-lint`.
_GO_COMMAND = re.compile(r"(?:^|[;&|]\s*)(?:\w+=\S*\s+)*go(?:\.exe)?(?:\s|$)")

_go_bin_dir: str | None = None
_injected = False
_hook_installed = False


def hermetic_go_bin_dir(ctx: Context) -> str:
    """Absolute path of `bin/` in the Bazel-managed Go SDK, i.e. GOROOT/bin."""
    global _go_bin_dir
    if _go_bin_dir is None:
        # cquery is idempotent: it fetches/extracts the SDK only if missing, and
        # resolving through the apparent repository name keeps us clear of Bazel's
        # (unstable) canonical repository naming.
        root = bazel(ctx, "cquery", "--output=files", GO_SDK_ROOT_LABEL, capture_output=True).strip()
        _go_bin_dir = str(Path(output_base(ctx), root).parent / "bin")
    return _go_bin_dir


def inject_go_toolchain(ctx: Context) -> str | None:
    """Prepend the Bazel-managed Go SDK to PATH, once per process, and return its bin/.

    PATH is mutated in place rather than passed per command so that processes we
    do not spawn ourselves — ninja, `subprocess.run`, `go generate` — inherit the
    same toolchain. Returns None when the SDK could not be located.
    """
    global _injected
    if not _injected:
        # Set before querying Bazel: that query runs commands through the hook below.
        _injected = True
        try:
            os.environ["PATH"] = hermetic_go_bin_dir(ctx) + os.pathsep + os.environ.get("PATH", "")
        except Exception as e:
            print(
                color_message(f"Could not locate the Bazel Go SDK ({e}), falling back to `go` from PATH", "orange"),
                file=sys.stderr,
            )
    return _go_bin_dir


def install_go_toolchain_hook() -> None:
    """Patch invoke's runner so that any task running `go` gets the Bazel SDK.

    Patching `Runner` rather than `Context` also covers the runners tasks build
    themselves (see `_handle_pipe_to_whydeadcode`). The lookup is deferred to the
    first `go` command so that tasks which never touch Go pay nothing.
    """
    global _hook_installed
    if _hook_installed:
        return
    _hook_installed = True

    run = Runner.run

    def run_with_go_toolchain(self, command, **kwargs):
        # MockContext has no real shell to query Bazel with.
        if not isinstance(self.context, MockContext) and _GO_COMMAND.search(command):
            bin_dir = inject_go_toolchain(self.context)
            env = kwargs.get("env")
            # invoke merges as dict(os.environ, **env), so a caller-supplied PATH
            # (the hermetic mingw one, for instance) would shadow the SDK.
            if bin_dir and env and env.get("PATH"):
                kwargs["env"] = {**env, "PATH": bin_dir + os.pathsep + env["PATH"]}
        return run(self, command, **kwargs)

    Runner.run = run_with_go_toolchain
