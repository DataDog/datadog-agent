"""Final-executable jemalloc linkage for the Invoke/Omnibus build path."""

from __future__ import annotations

import os
import shlex
import sys
from pathlib import Path

from tasks.libs.build.bazel import bazel
from tasks.libs.common.utils import get_repo_root
from tasks.libs.types.arch import Arch


def _go_quote(arg: str) -> str:
    # cmd/internal/quoted supports whole-argument quotes, not shell escaping.
    if not any(char.isspace() or char in "\"'" for char in arg):
        return arg
    for quote in ("'", '"'):
        if quote not in arg:
            return quote + arg + quote
    raise ValueError("Go linker arguments cannot contain both quote characters")


def append_extldflags(ldflags: str, flags: str) -> str:
    """Merge external flags into one Go linker option, preserving caller flags."""
    args = shlex.split(ldflags)
    result = []
    external = []
    iterator = iter(args)
    for arg in iterator:
        if arg == "-extldflags":
            external.append(next(iterator))
        elif arg.startswith("-extldflags="):
            external.append(arg.removeprefix("-extldflags="))
        else:
            result.append(arg)
    external.append(flags)
    result.append("-extldflags=" + " ".join(external))
    return " ".join(_go_quote(arg) for arg in result)


def with_native_allocator(ldflags: str, env: dict[str, str] | None) -> str:
    """Link only Linux Cgo products; never modify Cgo flags used to build DSOs."""
    effective_env = {**os.environ, **(env or {})}
    target_os = effective_env.get("GOOS") or {"win32": "windows"}.get(sys.platform, sys.platform)
    if target_os != "linux" or effective_env.get("CGO_ENABLED") == "0":
        return ldflags

    arch = Arch.from_str(effective_env.get("GOARCH") or "local")
    platform_arch = {"amd64": "x86_64", "arm64": "arm64"}[arch.go_arch]
    platform = f"--platforms=//bazel/platforms:linux_{platform_arch}"
    target = "//bazel/rules/native_allocator:archives"
    bazel("build", platform, target)
    outputs = bazel("cquery", platform, target, "--output=files", capture_output=True).splitlines()
    # CI disables convenience symlinks. Use the execution root, as the existing
    # component build helper does, rather than relying on workspace/bazel-out.
    execroot = bazel("info", "execution_root", capture_output=True).strip()
    archives = [Path(execroot, output).resolve() for output in outputs]
    if not archives or any(p.suffix not in {".a", ".lo"} or not p.is_file() for p in archives):
        raise ValueError(f"Expected built allocator static archives, got {outputs!r}")
    archive_flags = " ".join(_go_quote(str(p)) for p in archives)

    # -l would prefer the shared library. Root the public C API object and the
    # compatibility object without pulling jemalloc's optional C++ operators.
    # rtloader, Python, and native integrations must not embed their own copy.
    flags = (
        "-Wl,-u,malloc,-u,calloc,-u,realloc,-u,free,-u,malloc_usable_size,-u,mallctl,-u,pvalloc,-u,reallocarray "
        f"{archive_flags} -lm -lpthread -ldl -lc -lgcc "
        "-Wl,--export-dynamic -Wl,-z,lazy"
    )
    # The common flags already carry the map. Passing it twice makes GNU ld
    # reject an anonymous version script, so keep exactly one occurrence.
    if "--version-script=" not in ldflags:
        flags += " " + _go_quote(f"-Wl,--version-script={get_repo_root()}/datadog-agent.map")
    return append_extldflags(ldflags + " -linkmode=external", flags)
