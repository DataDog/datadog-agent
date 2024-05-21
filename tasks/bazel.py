"""
Bazel related tasks
"""

import os
import shutil
from glob import glob

from invoke import task


@task
def copy_prebuilt(ctx, rtloader_src, rtloader_build):
    """Copy separately prebuilt inputs for Bazel builds.

    This currently means copying rtloader.
    """
    prebuilt_rtloader_folder = os.path.join(os.path.dirname(__file__), "..", "bazel", "prebuilt_inputs", "rtloader")
    os.makedirs(prebuilt_rtloader_folder, exist_ok=True)
    # Copy .so files
    for src in glob(os.path.join(rtloader_build, "rtloader", "*.so*")):
        shutil.copy2(src, prebuilt_rtloader_folder)

    # Copy headers
    os.makedirs(os.path.join(prebuilt_rtloader_folder, "include"), exist_ok=True)
    os.makedirs(os.path.join(prebuilt_rtloader_folder, "common"), exist_ok=True)
    for src in glob(os.path.join(rtloader_src, "include", "*.h")):
        shutil.copy2(src, os.path.join(prebuilt_rtloader_folder, "include"))
    for src in glob(os.path.join(rtloader_src, "common", "*.h")):
        shutil.copy2(src, os.path.join(prebuilt_rtloader_folder, "common"))
