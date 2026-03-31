"""
Tasks for building Rust shared library checks.

The Rust checks under pkg/collector/sharedlibrary/rustchecks/ are compiled as
cdylib shared libraries (.so on Linux, .dll on Windows, .dylib on macOS).

E2E tests need platform-native binaries in
test/new-e2e/tests/agent-runtimes/checks/shared-library/files/.
This task builds them from source so they stay in sync with ABI changes.
"""

import platform
import shutil
import tempfile
from pathlib import Path

from invoke import Exit, task

from tasks.libs.common.color import Color, color_message

RUSTCHECKS_DIR = Path("pkg/collector/sharedlibrary/rustchecks")
E2E_FILES_DIR = Path("test/new-e2e/tests/agent-runtimes/checks/shared-library/files")

# Cargo package name -> desired output file stem (without platform extension)
CHECKS = {
    "example": "libdatadog-agent-example",
}


def _platform_info():
    """Return platform-specific shared library naming and tooling.

    Returns a dict with keys: ext, lib_prefix, cc_cmd.
    """
    system = platform.system()
    if system == "Linux":
        return {"ext": "so", "lib_prefix": "lib"}
    elif system == "Darwin":
        return {"ext": "dylib", "lib_prefix": "lib"}
    elif system == "Windows":
        return {"ext": "dll", "lib_prefix": ""}
    else:
        raise Exit(f"Unsupported platform: {system}")


def _build_no_run_symbol_lib(ctx, output_dir, pinfo):
    """Build a minimal shared library that deliberately lacks the check_run symbol.

    Used by E2E tests to verify the agent handles missing symbols gracefully.
    """
    dst = output_dir / f"{pinfo['lib_prefix']}datadog-agent-no-run-symbol.{pinfo['ext']}"

    with tempfile.TemporaryDirectory() as tmpdir:
        src = Path(tmpdir) / "empty.c"
        src.write_text("void _no_op(void) {}\n")
        if platform.system() == "Windows":
            ctx.run(f'cl /LD /Fe:"{dst}" "{src}"')
        else:
            ctx.run(f'cc -shared -o "{dst}" "{src}"')

    print(f"Built no-run-symbol library: {dst}")


@task(
    help={
        "release": "Build in release mode (default: True)",
    },
)
def build(ctx, release=True):
    """
    Build Rust shared library checks and copy them to the E2E test files directory.
    """
    pinfo = _platform_info()
    ext = pinfo["ext"]
    lib_prefix = pinfo["lib_prefix"]
    profile = "release" if release else "debug"
    profile_flag = f"--{profile}" if release else ""
    target_dir = RUSTCHECKS_DIR / "target" / profile

    # Build all check crates in a single cargo invocation
    pkg_flags = " ".join(f"--package {name}" for name in CHECKS)
    with ctx.cd(str(RUSTCHECKS_DIR)):
        ctx.run(f"cargo build {profile_flag} {pkg_flags}", pty=True)

    # Copy outputs to E2E test files directory
    E2E_FILES_DIR.mkdir(parents=True, exist_ok=True)
    for cargo_pkg, output_stem in CHECKS.items():
        src = target_dir / f"{lib_prefix}{cargo_pkg}.{ext}"
        dst = E2E_FILES_DIR / f"{output_stem}.{ext}"

        if not src.exists():
            raise Exit(
                color_message(f"Expected build output not found: {src}", Color.RED),
                code=1,
            )

        print(f"Copying {src} -> {dst}")
        shutil.copy2(src, dst)

    _build_no_run_symbol_lib(ctx, E2E_FILES_DIR, pinfo)
