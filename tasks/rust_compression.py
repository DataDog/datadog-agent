"""
Invoke tasks for building and testing the Rust compression library.
"""

import os

from invoke import task
from invoke.exceptions import Exit

RUST_DIR = os.path.join("pkg", "util", "compression", "rust")


def _check_rust_toolchain(ctx):
    """Check if Rust toolchain is available."""
    result = ctx.run("cargo --version", warn=True, hide=True)
    if result.failed:
        raise Exit(
            "Rust toolchain not found. Please install Rust: https://rustup.rs/",
            code=1,
        )


@task
def build(ctx, release=True, target=None):
    """
    Build the Rust compression library.

    Args:
        release: Build in release mode (default: True)
        target: Cross-compilation target (e.g., x86_64-unknown-linux-gnu)
    """
    _check_rust_toolchain(ctx)

    profile = "--release" if release else ""
    target_arg = f"--target {target}" if target else ""

    with ctx.cd(RUST_DIR):
        ctx.run(f"cargo build {profile} {target_arg}")

    print(f"Built Rust compression library in {'release' if release else 'debug'} mode")

    # Print library location
    mode = "release" if release else "debug"
    if target:
        lib_path = os.path.join(RUST_DIR, "target", target, mode)
    else:
        lib_path = os.path.join(RUST_DIR, "target", mode)

    print(f"Library location: {lib_path}")


@task
def test(ctx, release=False):
    """
    Run Rust compression library tests.

    Args:
        release: Run tests in release mode (default: False)
    """
    _check_rust_toolchain(ctx)

    profile = "--release" if release else ""

    with ctx.cd(RUST_DIR):
        ctx.run(f"cargo test {profile}")


@task
def bench(ctx):
    """Run Rust compression library benchmarks."""
    _check_rust_toolchain(ctx)

    with ctx.cd(RUST_DIR):
        ctx.run("cargo bench")


@task
def clean(ctx):
    """Clean the Rust compression library build artifacts."""
    _check_rust_toolchain(ctx)

    with ctx.cd(RUST_DIR):
        ctx.run("cargo clean")


@task
def fmt(ctx, check=False):
    """
    Format Rust code.

    Args:
        check: Check formatting without modifying files
    """
    _check_rust_toolchain(ctx)

    check_arg = "--check" if check else ""

    with ctx.cd(RUST_DIR):
        ctx.run(f"cargo fmt {check_arg}")


@task
def clippy(ctx):
    """Run Clippy linter on Rust code."""
    _check_rust_toolchain(ctx)

    with ctx.cd(RUST_DIR):
        ctx.run("cargo clippy -- -D warnings")


@task
def lint(ctx):
    """Run all Rust linters (fmt check + clippy)."""
    fmt(ctx, check=True)
    clippy(ctx)


@task
def doc(ctx, open_browser=False):
    """
    Generate Rust documentation.

    Args:
        open_browser: Open documentation in browser after generation
    """
    _check_rust_toolchain(ctx)

    open_arg = "--open" if open_browser else ""

    with ctx.cd(RUST_DIR):
        ctx.run(f"cargo doc {open_arg}")


@task
def verify_go_integration(ctx):
    """
    Verify Go integration compiles with the Rust library.

    This builds the Go code with the rust_compression build tag
    to verify the CGO integration works.
    """
    _check_rust_toolchain(ctx)

    # First build the Rust library
    build(ctx, release=True)

    # Then try to build Go code with the rust_compression tag
    print("Verifying Go integration...")
    result = ctx.run(
        "go build -tags 'rust_compression cgo' ./pkg/util/compression/impl-rust/...",
        warn=True,
    )

    if result.failed:
        print("Go integration verification failed!")
        print("This may be expected if the Rust library isn't fully linked.")
        print("Check the CGO flags in impl-rust/rust_strategy.go")
    else:
        print("Go integration verification passed!")


@task
def all(ctx):
    """Run full Rust build pipeline: lint, test, build."""
    lint(ctx)
    test(ctx)
    build(ctx, release=True)
    print("\nAll Rust compression tasks completed successfully!")
