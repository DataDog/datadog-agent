import os
import sys

from invoke import task

from tasks.rtloader import get_dev_path

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"

# Bazel builds libdd_sds from the dd-sensitive-data-scanner crate pinned in
# //:Cargo.toml, so there is no version to keep in sync here anymore.
SDS_SHARED_TARGET = "//deps/sds/rust:dd_sds_shared"


@task
def build_library(ctx):
    """
    Build the SDS shared library (libdd_sds) via Bazel and stage it next to the
    rtloader libs in dev/lib, so the non-bazel `agent.build --include-sds` and the
    source test jobs can link against it. Bazel is the single source of truth for
    the build (see //deps/sds/rust and //deps/sds:install for the omnibus path).
    """
    if is_windows:
        print("Not building the SDS library: unsupported on Windows.", file=sys.stderr)
        return

    ctx.run(f"bazel build {SDS_SHARED_TARGET}")
    bazel_bin = ctx.run("bazel info bazel-bin", hide=True).stdout.strip()

    lib_name = "libdd_sds.dylib" if is_darwin else "libdd_sds.so"
    src = os.path.join(bazel_bin, "deps", "sds", "rust", lib_name)

    dev_path = get_dev_path()
    lib_path = os.path.join(dev_path, "lib")
    lib64_path = os.path.join(dev_path, "lib64")

    # dev/lib must be a directory: this task may run before rtloader is built
    # (which normally creates it), and copying a file onto a missing dev/lib would
    # create dev/lib as a *file*, later breaking rtloader's CMake install.
    dests = [lib_path]
    if os.path.exists(lib64_path):
        dests.append(lib64_path)

    for dest in dests:
        os.makedirs(dest, exist_ok=True)
        ctx.run(f"cp {src} {dest}/")
        # Bazel outputs are read-only; make the copy writable so we can re-stamp it.
        ctx.run(f"chmod u+w {dest}/{lib_name}")
        if is_darwin:
            ctx.run(f"install_name_tool -id @rpath/{lib_name} {dest}/{lib_name}")
