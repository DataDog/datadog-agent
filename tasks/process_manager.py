"""
Process Manager tasks
"""

import os
import sys

from invoke import task
from invoke.exceptions import Exit

# constants
PROCMGR_BIN_PATH = os.path.join(".", "bin", "process-manager")
PROCMGR_DAEMON_PATH = os.path.join(".", "process_manager", "daemon")


@task
def build(ctx, release=True):
    """
    Build the Process Manager daemon (dd-procmgrd) using Cargo
    """
    if sys.platform == 'win32':
        print("Process Manager is not supported on Windows")
        raise Exit(code=1)

    # Check if cargo is available
    result = ctx.run("cargo --version", warn=True, hide=True)
    if result.exited != 0:
        print("Error: cargo not found. Please install Rust toolchain.")
        print("Visit https://rustup.rs/ for installation instructions.")
        raise Exit(code=1)

    # Build the Rust binary
    build_mode = "--release" if release else ""
    cmd = f"cargo build {build_mode} --manifest-path={PROCMGR_DAEMON_PATH}/Cargo.toml"

    print(f"Building process manager daemon: {cmd}")
    ctx.run(cmd)

    # Create bin directory if it doesn't exist
    os.makedirs(PROCMGR_BIN_PATH, exist_ok=True)

    # Copy the binary to the bin directory
    build_dir = "release" if release else "debug"
    source_bin = os.path.join(PROCMGR_DAEMON_PATH, "target", build_dir, "dd-procmgrd")
    dest_bin = os.path.join(PROCMGR_BIN_PATH, "dd-procmgrd")

    if os.path.exists(source_bin):
        ctx.run(f"cp {source_bin} {dest_bin}")
        print(f"Binary copied to: {dest_bin}")
    else:
        print(f"Error: Built binary not found at {source_bin}")
        raise Exit(code=1)


@task
def clean(ctx):
    """
    Clean the Process Manager build artifacts
    """
    print("Cleaning process manager build artifacts...")
    ctx.run(f"cargo clean --manifest-path={PROCMGR_DAEMON_PATH}/Cargo.toml", warn=True)

    # Remove copied binary
    dest_bin = os.path.join(PROCMGR_BIN_PATH, "dd-procmgrd")
    if os.path.exists(dest_bin):
        os.remove(dest_bin)
        print(f"Removed: {dest_bin}")


@task
def run(ctx):
    """
    Build and run the Process Manager daemon
    """
    build(ctx)
    binary_path = os.path.join(PROCMGR_BIN_PATH, "dd-procmgrd")
    print(f"Running process manager: {binary_path}")
    ctx.run(binary_path)
