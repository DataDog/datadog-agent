import os
import sys
import tempfile

from invoke import task

from tasks.libs.common.utils import collapsed_section
from tasks.rtloader import get_dev_path

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"

sds_version = "v0.1.2"


@task
def build_library(ctx):
    """
    Build the SDS shared library
    """
    if is_windows:
        print("Not building the SDS library: unsupported on Windows.", file=sys.stderr)
        return
    with collapsed_section("Clone and build SDS"):
        with tempfile.TemporaryDirectory() as temp_dir:
            with ctx.cd(temp_dir):
                ctx.run("git clone https://github.com/DataDog/dd-sensitive-data-scanner", err_stream=sys.stdout)
                with ctx.cd("dd-sensitive-data-scanner/sds-go/rust"):
                    ctx.run(f"git checkout {sds_version}", err_stream=sys.stdout)
                    ctx.run("cargo build --release", err_stream=sys.stdout)
                    # write the lib besides rtloader libs
                    dev_path = get_dev_path()
                    lib_path = os.path.join(dev_path, "lib")
                    lib64_path = os.path.join(dev_path, "lib64")
                    # We do not support Windows for now.
                    if is_darwin:
                        ctx.run(f"cp target/release/libsds_go.dylib {lib_path}")
                        if os.path.exists(lib64_path):
                            ctx.run(f"cp target/release/libsds_go.dylib {lib64_path}")
                    else:
                        ctx.run(f"cp target/release/libsds_go.so {lib_path}")
                        if os.path.exists(lib64_path):
                            ctx.run(f"cp target/release/libsds_go.so {lib64_path}")
