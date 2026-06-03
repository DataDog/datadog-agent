"""
Invoke tasks to build the Sensitive Data Scanner (SDS) shared library.

The library lives in https://github.com/DataDog/dd-sensitive-data-scanner and is
built from its Rust sources. The resulting shared library is required to build
and test the Agent with the `sds` build tag (see pkg/util/sds).
"""

import os
import sys
import tempfile

from invoke import task

from tasks.rtloader import get_dev_path

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"

# Pinned to the same commit as the github.com/DataDog/dd-sensitive-data-scanner/sds-go/go
# module required in pkg/util/sds/go.mod. Keep both in sync.
SDS_VERSION = "4d0ef6614dd4"

# Name of the shared library produced by `cargo build` (matches the `-ldd_sds_go`
# cgo directive in the sds-go Go bindings).
LIB_BASENAME = "libdd_sds_go"


@task
def build_library(ctx):
    """
    Build the SDS shared library and install it next to the rtloader libs (dev/lib).
    """
    if is_windows:
        print("Not building the SDS library: unsupported on Windows.", file=sys.stderr)
        return

    lib_filename = f"{LIB_BASENAME}.dylib" if is_darwin else f"{LIB_BASENAME}.so"

    with tempfile.TemporaryDirectory() as temp_dir:
        with ctx.cd(temp_dir):
            ctx.run("git clone https://github.com/DataDog/dd-sensitive-data-scanner")
            with ctx.cd("dd-sensitive-data-scanner"):
                ctx.run(f"git checkout {SDS_VERSION}")
            with ctx.cd("dd-sensitive-data-scanner/sds-go/rust"):
                ctx.run("cargo build --release")
                # install the lib besides the rtloader libs so the Agent build links against it
                dev_path = get_dev_path()
                lib_path = os.path.join(dev_path, "lib")
                lib64_path = os.path.join(dev_path, "lib64")
                ctx.run(f"mkdir -p {lib_path}")
                ctx.run(f"cp target/release/{lib_filename} {lib_path}")
                if os.path.exists(lib64_path):
                    ctx.run(f"cp target/release/{lib_filename} {lib64_path}")
