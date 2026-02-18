"""
Rust patterns library integration tasks
"""

import os
import shutil
import sys
import tempfile

from invoke import task

from tasks.libs.common.utils import REPO_PATH
from tasks.rtloader import get_dev_path


def _detect_dd_source(dd_source_path):
    """Auto-detect or validate the dd-source path."""
    if not dd_source_path:
        agent_path = REPO_PATH
        candidate1 = os.path.join(os.path.dirname(agent_path), "dd-source")
        candidate2 = os.path.join(os.path.dirname(os.path.dirname(agent_path)), "dd-source")
        candidate3 = os.path.abspath("../dd-source")

        # Also check DD_SOURCE_PATH env var
        env_path = os.environ.get('DD_SOURCE_PATH')
        candidates = [c for c in [env_path, candidate1, candidate2, candidate3] if c]

        for candidate in candidates:
            if os.path.exists(candidate):
                dd_source_path = candidate
                break

        if not dd_source_path:
            raise RuntimeError(
                "Could not auto-detect dd-source. Tried:\n"
                + "\n".join(f"  - {c}" for c in candidates)
                + "\nPlease specify the correct path with --dd-source-path"
            )

    dd_source_path = os.path.abspath(dd_source_path)

    if not os.path.exists(dd_source_path):
        raise RuntimeError(
            f"dd-source not found at {dd_source_path}\nPlease specify the correct path with --dd-source-path"
        )

    patterns_dir = "domains/data_science/libs/rust/patterns"
    patterns_path = os.path.join(dd_source_path, patterns_dir)

    if not os.path.exists(patterns_path):
        raise RuntimeError(
            f"Rust patterns library not found at {patterns_path}\n"
            f"Make sure you're using a dd-source checkout with the patterns library"
        )

    return dd_source_path, patterns_dir


def _get_lib_file():
    """Return the platform-specific library filename."""
    if sys.platform == "darwin":
        return "libpatterns.dylib"
    elif sys.platform == "win32":
        return "patterns.dll"
    else:
        return "libpatterns.so"


def _copy_artifacts(ctx, lib_source, header_source, lib_file):
    """Copy built library and header into dev/lib."""
    dev_path = get_dev_path()
    lib_path = os.path.join(dev_path, "lib")
    os.makedirs(lib_path, exist_ok=True)

    lib_dest = os.path.join(lib_path, lib_file)
    header_dest = os.path.join(lib_path, "libpatterns.h")

    if not os.path.exists(lib_source):
        raise RuntimeError(f"Built library not found at {lib_source}")
    if not os.path.exists(header_source):
        raise RuntimeError(f"Generated header not found at {header_source}")

    # Remove existing files first (they may be read-only from Bazel or owned by root)
    for dest in [lib_dest, header_dest]:
        if os.path.exists(dest):
            try:
                os.chmod(dest, 0o644)
                os.remove(dest)
            except PermissionError:
                # File may be owned by root (e.g. from a previous container build)
                ctx.run(f"sudo rm -f {dest}", echo=True)

    print(f"→ Copying {lib_file} to {lib_dest}")
    shutil.copy(lib_source, lib_dest)

    if sys.platform == "darwin":
        print(f"→ Fixing install name for {lib_file}")
        ctx.run(f"install_name_tool -id @rpath/{lib_file} {lib_dest}", echo=False)

    print(f"→ Copying libpatterns.h to {header_dest}")
    shutil.copy(header_source, header_dest)

    return lib_dest, header_dest


def _build_with_bazel(ctx, dd_source_path, patterns_dir):
    """Build using Bazel (requires internal network access)."""
    with ctx.cd(dd_source_path):
        print("→ Building libpatterns_c (bazel)...")
        ctx.run(f"bzl build //{patterns_dir}:libpatterns_c", echo=True)
        print("→ Generating C header (bazel)...")
        ctx.run(f"bzl build //{patterns_dir}:cbindgen", echo=True)

    bazel_bin = os.path.join(dd_source_path, f"bazel-bin/{patterns_dir}")
    lib_file = _get_lib_file()
    lib_source = os.path.join(bazel_bin, lib_file)
    header_source = os.path.join(bazel_bin, "include/libpatterns.h")
    return lib_source, header_source, lib_file


def _build_with_cargo(ctx, dd_source_path, patterns_dir):
    """Build using Cargo in isolation (no internal network required, only crates.io).

    The dd-source workspace configures a custom Cargo registry
    (depot-read-api-rust.us1.ddbuild.io) that requires VPN/internal network.
    To avoid this, we copy just the patterns crate to an isolated directory
    and build it as a standalone crate with only crates.io dependencies.
    """
    patterns_path = os.path.join(dd_source_path, patterns_dir)

    # Check that Rust/Cargo is installed
    result = ctx.run("rustc --version", warn=True, hide=True)
    if not result or result.failed:
        print("Rust not found. Installing via rustup...")
        ctx.run("curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y", echo=True)
        ctx.run("echo 'source $HOME/.cargo/env' >> $HOME/.bashrc", warn=True, hide=True)
        os.environ["PATH"] = os.path.expanduser("~/.cargo/bin") + ":" + os.environ.get("PATH", "")

    # Build in an isolated directory to avoid the dd-source workspace's
    # custom Cargo registry which requires internal network access.
    build_dir = os.path.join(tempfile.gettempdir(), "patterns-cargo-build")
    if os.path.exists(build_dir):
        shutil.rmtree(build_dir)

    print(f"→ Copying patterns crate to {build_dir} (isolated from workspace)...")
    shutil.copytree(patterns_path, build_dir, ignore=shutil.ignore_patterns("target", ".git"))

    # Rewrite Cargo.toml to replace workspace references with concrete values.
    # The original uses `version.workspace = true`, `edition.workspace = true`,
    # `serde_json = { workspace = true }`, etc. which require the workspace root.
    cargo_toml_path = os.path.join(build_dir, "Cargo.toml")
    standalone_toml = """\
[package]
name = "patterns"
version = "0.1.0"
edition = "2024"

[lib]
crate-type = ["lib", "cdylib"]

[dependencies]
regex-automata = { version = "0.4", features = ["dfa-build", "dfa-search"] }
serde = { version = "1.0", features = ["derive"] }
serde_json = { version = "1.0", features = ["preserve_order", "raw_value"] }
thiserror = "1.0.69"
once_cell = "1.19"
flatbuffers = "24.3"

[dev-dependencies]
anyhow = "1.0"
criterion = "0.5"

[[bench]]
name = "signature_performance"
harness = false
"""
    with open(cargo_toml_path, "w") as f:
        f.write(standalone_toml)

    # Build the cdylib
    print("→ Building libpatterns (cargo, isolated from workspace)...")
    with ctx.cd(build_dir):
        ctx.run("cargo build --release", echo=True)

    # Generate C header with cbindgen
    result = ctx.run("cbindgen --version", warn=True, hide=True)
    if not result or result.failed:
        print("→ Installing cbindgen...")
        ctx.run("cargo install cbindgen", echo=True)

    print("→ Generating C header (cbindgen)...")
    header_out = os.path.join(build_dir, "target", "release", "libpatterns.h")
    with ctx.cd(build_dir):
        ctx.run(
            f"cbindgen --config cbindgen.toml --crate patterns --output {header_out}",
            echo=True,
        )

    lib_file = _get_lib_file()
    lib_source = os.path.join(build_dir, "target", "release", lib_file)
    return lib_source, header_out, lib_file


@task
def setup_local_library(ctx, dd_source_path=None, cargo=False):
    """
    Setup local Rust patterns library for development.

    This task builds the Rust patterns library from a local dd-source checkout
    and copies it to the Agent's dev lib directory.

    Usage:
        # Build with Bazel (default, requires internal network)
        dda inv patterns.setup-local-library --dd-source-path=/path/to/dd-source

        # Build with Cargo (works in devcontainers without VPN)
        dda inv patterns.setup-local-library --dd-source-path=/dd-source --cargo

    Args:
        dd_source_path: Path to dd-source repository (optional, auto-detected)
        cargo: Use cargo instead of Bazel to build (default: False)
    """
    dd_source_path, patterns_dir = _detect_dd_source(dd_source_path)
    print(f"Building Rust patterns library from {dd_source_path}")

    if cargo:
        lib_source, header_source, lib_file = _build_with_cargo(ctx, dd_source_path, patterns_dir)
    else:
        lib_source, header_source, lib_file = _build_with_bazel(ctx, dd_source_path, patterns_dir)

    lib_dest, header_dest = _copy_artifacts(ctx, lib_source, header_source, lib_file)

    print("")
    print("✅ Rust patterns library setup complete!")
    print(f"   Library: {lib_dest}")
    print(f"   Header:  {header_dest}")
    print("")
    print("Next steps:")
    print("  1. Build the agent with Rust patterns:")
    print("     dda inv agent.build --build-include=rust_patterns")
    print("")
    print("  2. Run tests:")
    print("     cd pkg/logs/patterns/token && go test -v -tags rust_patterns")
    print("")
    print("  3. Run the agent:")
    print("     ./bin/agent/agent run -c bin/agent/dist/datadog.yaml")


@task
def build_library(ctx, dd_source_ref=None, use_local=True, force=False):
    """
    Build Rust patterns library from dd-source.

    This task is designed to work in both local dev and CI environments:
    - Local dev: Uses local dd-source checkout
    - CI: Clones dd-source from git

    Usage:
        # Local development (uses local dd-source)
        dda inv patterns.build-library

        # CI (clones dd-source from specific branch/commit)
        dda inv patterns.build-library --dd-source-ref=rust-patterns --no-use-local

        # Force rebuild even if library exists
        dda inv patterns.build-library --force

    Args:
        dd_source_ref: Git ref (branch/tag/commit) to checkout (for CI)
        use_local: Use local dd-source if available (default: True)
        force: Force rebuild even if library already exists (default: False)
    """
    # Check if library already exists (skip rebuild unless forced)
    if not force:
        dev_path = get_dev_path()
        lib_path = os.path.join(dev_path, "lib")

        if sys.platform == "darwin":
            lib_file = "libpatterns.dylib"
        elif sys.platform == "win32":
            lib_file = "patterns.dll"
        else:
            lib_file = "libpatterns.so"

        lib_dest = os.path.join(lib_path, lib_file)
        header_dest = os.path.join(lib_path, "libpatterns.h")

        if os.path.exists(lib_dest) and os.path.exists(header_dest):
            print(f"Rust patterns library already exists at {lib_dest}")
            print("Skipping rebuild. Use --force to rebuild.")
            return

    if use_local:
        # Try local first (dev environment)
        local_dd_source = os.path.join(os.path.dirname(REPO_PATH), "dd-source")
        if os.path.exists(local_dd_source):
            print(f"Using local dd-source at {local_dd_source}")
            setup_local_library(ctx, dd_source_path=local_dd_source)
            return

    # Clone for CI
    if not dd_source_ref:
        dd_source_ref = os.environ.get('DD_SOURCE_REF', 'main')

    print(f"Cloning dd-source (ref: {dd_source_ref})")

    with tempfile.TemporaryDirectory() as temp_dir:
        dd_source_path = os.path.join(temp_dir, "dd-source")

        with ctx.cd(temp_dir):
            ctx.run("git clone https://github.com/DataDog/dd-source", echo=True)

        with ctx.cd(dd_source_path):
            ctx.run(f"git checkout {dd_source_ref}", echo=True)

        # Use the cloned dd-source
        setup_local_library(ctx, dd_source_path=dd_source_path)


@task
def clean(ctx):
    """
    Clean the Rust patterns library from dev environment.

    Usage:
        dda inv patterns.clean
    """
    dev_path = get_dev_path()
    lib_path = os.path.join(dev_path, "lib")

    if sys.platform == "darwin":
        lib_file = "libpatterns.dylib"
    elif sys.platform == "win32":
        lib_file = "patterns.dll"
    else:
        lib_file = "libpatterns.so"

    lib_dest = os.path.join(lib_path, lib_file)
    header_dest = os.path.join(lib_path, "libpatterns.h")

    removed = []

    if os.path.exists(lib_dest):
        os.remove(lib_dest)
        removed.append(lib_dest)

    if os.path.exists(header_dest):
        os.remove(header_dest)
        removed.append(header_dest)

    if removed:
        print("Removed:")
        for path in removed:
            print(f"  - {path}")
    else:
        print("Nothing to clean (library not found)")
