import os
import sys
import tempfile

from invoke import task

from tasks.rtloader import get_dev_path

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"


@task
def build_library(ctx):
    """
    Build the SDS shared library
    """
    if is_windows:
        print("not supported")
        return
    with tempfile.TemporaryDirectory() as temp_dir:
        with ctx.cd(temp_dir):
            ctx.run("git clone https://github.com/DataDog/dd-sensitive-data-scanner")
            with ctx.cd("dd-sensitive-data-scanner/sds-go/rust"):
                ctx.run("cargo build --release")
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
