"""
Invoke tasks for building Rust-based shared-library checks.

These checks compile to `cdylib` and must be staged into `checks.d` with the
expected loader naming convention:
  libdatadog-agent-<check_id>.so
"""

from __future__ import annotations

import os
import shutil
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.build.bazel import bazel
from tasks.libs.common.utils import gitlab_section

RUSTCHECKS_MANIFEST_REL_PATH = "pkg/collector/sharedlibrary/rustchecks/shared_checks_manifest.yaml"


@dataclass(frozen=True)
class CheckSpec:
    id: str
    crate: str
    include_in_build: bool
    platforms: list[str]


def _repo_root(ctx) -> Path:
    repo_root = ctx.run("git rev-parse --show-toplevel", hide=True).stdout.strip()
    return Path(repo_root)


def _current_platform() -> str:
    if sys.platform.startswith("linux"):
        return "linux"
    return "unsupported"


def _load_manifest(manifest_path: Path) -> list[CheckSpec]:
    try:
        payload: dict[str, Any] = yaml.safe_load(manifest_path.read_text(encoding="utf-8"))
    except FileNotFoundError as e:
        raise Exit(f"Rust shared checks manifest not found: {manifest_path}") from e

    if not payload:
        raise Exit(f"Rust shared checks manifest is empty: {manifest_path}")

    checks = payload.get("checks", [])
    specs: list[CheckSpec] = []
    for c in checks:
        specs.append(
            CheckSpec(
                id=c["id"],
                crate=c["crate"],
                include_in_build=bool(c.get("include_in_build", False)),
                platforms=list(c.get("platforms", [])),
            )
        )
    return specs


def _select_checks(specs: list[CheckSpec], platform: str) -> tuple[list[CheckSpec], list[str]]:
    final: list[CheckSpec] = []
    skipped: list[str] = []
    for s in specs:
        if not s.include_in_build:
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
    """Build and stage Rust shared-library checks into the provided `checks.d` dir."""

    if not checks_d_dir:
        raise Exit("Missing required argument: checks_d_dir")

    checks_d_path = Path(checks_d_dir)
    repo_root = _repo_root(ctx)

    default_manifest_path = repo_root / RUSTCHECKS_MANIFEST_REL_PATH
    manifest = Path(manifest_path) if manifest_path else default_manifest_path

    platform = _current_platform()
    if platform != "linux":
        raise Exit("Refusing to build Rust shared-library checks outside linux")

    specs = _load_manifest(manifest)
    selected, skipped = _select_checks(specs, platform)

    if skipped:
        print(f"Skipping checks not supported on this platform: {skipped}")

    if not selected:
        print("No Rust shared-library checks selected; leaving checks.d untouched.")
        return

    checks_d_path.mkdir(parents=True, exist_ok=True)

    # Drop stale libs for manifest checks so deselected ones are not shipped.
    managed_ids = {s.id for s in specs}
    for check_id in managed_ids:
        candidate = checks_d_path / f"libdatadog-agent-{check_id}.so"
        if candidate.exists():
            candidate.unlink()

    # Build selected crates via Bazel-wrapped cargo in the rustchecks workspace.
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

    bazel_run_args = [
        "run",
        "--remote_download_outputs=all",
        f"--action_env=CARGO_HOME={build_env['CARGO_HOME']}",
    ]

    with gitlab_section("Build Rust shared-library checks", collapsed=True):
        bazel(ctx, *bazel_run_args, *cargo_args)

    # Stage built libs with loader naming and owner-only perms.
    target_mode = 0o500
    for s in selected:
        built_lib = rustchecks_dir / "target" / "release" / f"lib{s.crate}.so"
        if not built_lib.exists():
            raise Exit(f"Expected built library not found: {built_lib}")

        staged_lib = checks_d_path / f"libdatadog-agent-{s.id}.so"
        shutil.copy2(built_lib, staged_lib)
        os.chmod(staged_lib, target_mode)

        print(f"Staged {staged_lib}")
