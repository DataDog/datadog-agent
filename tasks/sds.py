import contextlib
import glob
import json
import os
import platform
import re
import shutil
import string
import sys
import tarfile
import tempfile
from pathlib import Path
from subprocess import check_output

import requests
from invoke import task
from invoke.exceptions import Exit

is_windows = sys.platform == "win32"
is_darwin = sys.platform == "darwin"

from tasks.agent import BUNDLED_AGENTS
from tasks.rtloader import get_dev_path

@task
def build_sds_library(ctx, branch="main"):
    if is_windows:
        printf("not supported")
        return
    with tempfile.TemporaryDirectory() as temp_dir:
        with ctx.cd(temp_dir):
            ctx.run(f"git clone https://github.com/DataDog/dd-sensitive-data-scanner")
            with ctx.cd("dd-sensitive-data-scanner/sds-go/rust"):
                ctx.run(f"cargo build --release")
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

