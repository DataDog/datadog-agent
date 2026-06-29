import os
import sys

from invoke import task

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"

SDS_SHARED_TARGET = "//deps/sds/rust:dd_sds_shared"


def _dev_lib_path():
    here = os.path.abspath(os.path.dirname(__file__))
    return os.path.abspath(os.path.join(here, '..', 'dev', 'lib'))


@task
def build_library(ctx):
    """
    Build the SDS shared library (libdd_sds) via Bazel and stage it under dev/lib
    so `agent.build --include-sds` and the source test jobs can link against it.
    """
    if is_windows:
        print("Not building the SDS library: unsupported on Windows.", file=sys.stderr)
        return

    ctx.run(f"bazel build {SDS_SHARED_TARGET}")
    bazel_bin = ctx.run("bazel info bazel-bin", hide=True).stdout.strip()

    lib_name = "libdd_sds.dylib" if is_darwin else "libdd_sds.so"
    src = os.path.join(bazel_bin, "deps", "sds", "rust", lib_name)

    dest = _dev_lib_path()
    os.makedirs(dest, exist_ok=True)
    ctx.run(f"cp {src} {dest}/")
    # Bazel outputs are read-only; make the copy writable to re-stamp it.
    ctx.run(f"chmod u+w {dest}/{lib_name}")
    if is_darwin:
        ctx.run(f"install_name_tool -id @rpath/{lib_name} {dest}/{lib_name}")
