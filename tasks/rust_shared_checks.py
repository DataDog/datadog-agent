"""
Invoke tasks for building Rust-based shared-library checks.

These checks compile to `cdylib` and must be staged into `checks.d` with the
expected loader naming convention:
  libdatadog-agent-<check_id>.(so|dylib)
"""

from __future__ import annotations

import json
import os
import shutil
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.build.bazel import bazel
from tasks.libs.common.utils import gitlab_section

RUSTCHECKS_MANIFEST_REL_PATH = "pkg/collector/sharedlibrary/rustchecks/shared_checks_manifest.json"


@dataclass(frozen=True)
class CheckSpec:
    id: str
    crate: str
    default: bool
    platforms: list[str]


def _repo_root(ctx) -> Path:
    repo_root = ctx.run("git rev-parse --show-toplevel", hide=True).stdout.strip()
    return Path(repo_root)


def _current_platform() -> str:
    if sys.platform.startswith("linux"):
        return "linux"
    if sys.platform == "darwin":
        return "darwin"
    return "windows"


def _lib_extension(platform: str) -> str:
    if platform == "darwin":
        return "dylib"
    # Treat everything else as ELF-like for our purposes (we gate out windows).
    return "so"


def _parse_csv_env(var_name: str) -> set[str]:
    raw = os.environ.get(var_name, "")
    if not raw.strip():
        return set()
    return {v.strip() for v in raw.split(",") if v.strip()}


def _env_bool(var_name: str) -> bool:
    return os.environ.get(var_name, "").lower() in {"1", "true", "yes", "y", "on"}


def _load_manifest(manifest_path: Path) -> list[CheckSpec]:
    try:
        payload: dict[str, Any] = json.loads(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError as e:
        raise Exit(f"Rust shared checks manifest not found: {manifest_path}") from e

    checks = payload.get("checks", [])
    specs: list[CheckSpec] = []
    for c in checks:
        specs.append(
            CheckSpec(
                id=c["id"],
                crate=c["crate"],
                default=bool(c.get("default", False)),
                platforms=list(c.get("platforms", [])),
            )
        )
    return specs


def _select_checks(specs: list[CheckSpec], platform: str) -> tuple[list[CheckSpec], list[str]]:
    include = _parse_csv_env("DD_RUST_SHARED_CHECKS_INCLUDE")
    exclude = _parse_csv_env("DD_RUST_SHARED_CHECKS_EXCLUDE")

    # Convenience aliases.
    if _env_bool("DD_RUST_SHARED_CHECKS_INCLUDE_EXAMPLE") or _env_bool("DD_RUST_SHARED_CHECKS_ENABLE_EXAMPLE"):
        include.add("example")

    selected_ids = {s.id for s in specs if s.default}
    selected_ids |= include
    selected_ids -= exclude

    final: list[CheckSpec] = []
    skipped: list[str] = []
    for s in specs:
        if s.id not in selected_ids:
            continue

        if platform not in s.platforms:
            skipped.append(f"{s.id} (platform={platform})")
            continue

        final.append(s)

    return final, skipped


def _cargo_build_env(repo_root: Path) -> dict[str, str]:
    cargo_home = repo_root / "pkg/collector/sharedlibrary/rustchecks/.cargo-home"
    cargo_home.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["CARGO_HOME"] = str(cargo_home)
    return env


@task
def build(ctx, checks_d_dir, manifest_path=None):
    """
    Build and stage Rust shared-library checks into the provided `checks.d` dir.

    Environment configuration:
      - `DD_RUST_SHARED_CHECKS_INCLUDE` (comma-separated ids)
      - `DD_RUST_SHARED_CHECKS_EXCLUDE` (comma-separated ids)
      - `DD_RUST_SHARED_CHECKS_INCLUDE_EXAMPLE` / `DD_RUST_SHARED_CHECKS_ENABLE_EXAMPLE` (true/1)
    """

    if not checks_d_dir:
        raise Exit("Missing required argument: checks_d_dir")

    checks_d_path = Path(checks_d_dir)
    repo_root = _repo_root(ctx)

    default_manifest_path = repo_root / RUSTCHECKS_MANIFEST_REL_PATH
    manifest = Path(manifest_path) if manifest_path else default_manifest_path

    platform = _current_platform()
    if platform == "windows":
        raise Exit("Refusing to build Rust shared-library checks on windows")

    specs = _load_manifest(manifest)
    selected, skipped = _select_checks(specs, platform)

    if skipped:
        print(f"Skipping checks not supported on this platform: {skipped}")

    if not selected:
        print("No Rust shared-library checks selected; leaving checks.d untouched.")
        return

    # Ensure directory exists before staging.
    checks_d_path.mkdir(parents=True, exist_ok=True)

    ext = _lib_extension(platform)

    # Clean up any previously staged libraries for the checks managed by this task.
    managed_ids = {s.id for s in specs}
    for check_id in managed_ids:
        candidate = checks_d_path / f"libdatadog-agent-{check_id}.{ext}"
        if candidate.exists():
            candidate.unlink()

    rustchecks_dir = repo_root / "pkg/collector/sharedlibrary/rustchecks"
    build_env = _cargo_build_env(repo_root)
    cargo_args = [
        "@rules_rust//tools/upstream_wrapper:cargo",
        "--",
        "build",
        "--release",
        "--manifest-path",
        str(rustchecks_dir / "Cargo.toml"),
    ]
    for s in selected:
        cargo_args += ["-p", s.crate]

    with gitlab_section("Build Rust shared-library checks", collapsed=True):
        bazel(
            ctx,
            "run",
            f"--action_env=CARGO_HOME={build_env['CARGO_HOME']}",
            *cargo_args,
        )

    # Stage and enforce file mode:
    #   - group/other must have no permissions (loader uses mode bits checks)
    #   - owner must have read+execute so dlopen can access the library
    #
    # This results in 0500 (r-x------).
    target_mode = 0o500
    for s in selected:
        built_lib = rustchecks_dir / "target" / "release" / f"lib{s.crate}.{ext}"
        if not built_lib.exists():
            raise Exit(f"Expected built library not found: {built_lib}")

        staged_lib = checks_d_path / f"libdatadog-agent-{s.id}.{ext}"
        shutil.copy2(built_lib, staged_lib)
        os.chmod(staged_lib, target_mode)

        print(f"Staged {staged_lib}")
