"""
customaction namespaced tasks
"""


import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from .utils import get_version_numeric_only

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

    ver = get_version_numeric_only(ctx, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")
    verprops = f" /p:MAJ_VER={build_maj} /p:MIN_VER={build_min} /p:PATCH_VER={build_patch} "
    print(f"arch is {arch}")
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
        vs_env_bat = f'{vsroot}\\VC\\Auxiliary\\Build\\{batchfile}'
        cmd = f'call "{vs_env_bat}" && msbuild {CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\install-cmd.vcxproj /p:Configuration={configuration} /p:Platform={arch}'
    else:
        cmd = f'msbuild {CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\install-cmd.vcxproj /p:Configuration={configuration} /p:Platform={arch}'

    cmd += verprops
    print(f"Build Command: {cmd}")

    ctx.run(cmd)
    srcdll = None
    if arch is not None and arch == "x86":
        srcdll = f"{CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\{configuration}\\install-cmd.exe"
    else:
        srcdll = f"{CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\x64\\{configuration}\\install-cmd.exe"
    shutil.copy2(srcdll, BIN_PATH)


@task
def clean(_, arch="x64", debug=False):
    configuration = "Release"
    if debug:
        configuration = "Debug"

    if arch is not None and arch == "x86":
        srcdll = f"{CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\{configuration}"
    else:
        srcdll = f"{CUSTOM_ACTION_ROOT_DIR}\\install-cmd\\x64\\{configuration}"
    shutil.rmtree(srcdll, BIN_PATH)
