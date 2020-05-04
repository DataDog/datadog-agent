"""
customaction namespaced tasks
"""
from __future__ import print_function
import glob
import os
import shutil
import sys
import platform
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name, get_build_flags, get_version_numeric_only, load_release_versions
from .utils import REPO_PATH
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS
from .go import deps

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"
CUSTOM_ACTION_ROOT_DIR = "tools\\windows\\install-help"

@task
def build(ctx, major_version='7', vstudio_root=None, arch="x64", debug=False):
    """
    Build the custom action library for the agent
    """

    if sys.platform != 'win32':
        print("Custom action library is only for Win32")
        raise Exit(code=1)

    ver = get_version_numeric_only(ctx, env=os.environ, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")
    verprops = " /p:MAJ_VER={build_maj} /p:MIN_VER={build_min} /p:PATCH_VER={build_patch} ".format(
            build_maj=build_maj,
            build_min=build_min,
            build_patch=build_patch
        )
    print("arch is {}".format(arch))
    cmd = ""
    configuration = "Release"
    if debug:
        configuration = "Debug"

    if not os.getenv("VCINSTALLDIR"):
        print("VC Not installed in environment; checking other locations")

        vsroot = vstudio_root or os.getenv('VSTUDIO_ROOT')
        if not vsroot:
            print("Must have visual studio installed")
            raise Exit(code=2)
        batchfile = "vcvars64.bat"
        if arch == "x86":
            batchfile = "vcvars32.bat"
        vs_env_bat = '{}\\VC\\Auxiliary\\Build\\{}'.format(vsroot, batchfile)
        cmd = 'call \"{}\" && msbuild {}\\uninstall-cmd\\uninstall-cmd.vcxproj /p:Configuration={} /p:Platform={}'.format(
            vs_env_bat, CUSTOM_ACTION_ROOT_DIR, configuration, arch)
    else:
        cmd = 'msbuild {}\\uninstall-cmd\\uninstall-cmd.vcxproj /p:Configuration={} /p:Platform={}'.format(
            CUSTOM_ACTION_ROOT_DIR, configuration, arch)

    cmd += verprops
    print("Build Command: %s" % cmd)

    ctx.run(cmd)
    srcdll = None
    if arch is not None and arch == "x86":
        srcdll = "{}\\uninstall-cmd\\{}\\uninstall-cmd.exe".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    else:
        srcdll = "{}\\uninstall-cmd\\x64\\{}\\uninstall-cmd.exe".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    shutil.copy2(srcdll, BIN_PATH)

@task
def clean(ctx, arch="x64", debug=False):
    configuration = "Release"
    if debug:
        configuration = "Debug"

    if arch is not None and arch == "x86":
        srcdll = "{}\\uninstall-cmd\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    else:
        srcdll = "{}\\uninstall-cmd\\x64\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    shutil.rmtree(srcdll, BIN_PATH)

