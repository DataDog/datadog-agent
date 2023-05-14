"""
customaction namespaced tasks
"""


import glob
import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from .libs.common.color import color_message
from .utils import get_version, get_version_numeric_only

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"
CUSTOM_ACTION_ROOT_DIR = "tools\\windows\\install-help"


def try_run(ctx, cmd, n):
    """
    Tries to run a command n number of times. If after n tries it still
    fails, returns False.
    """

    for _ in range(n):
        res = ctx.run(cmd, warn=True)
        if res.exited is None or res.exited > 0:
            print(
                color_message(
                    f"Failed to run \"{cmd}\" - retrying",
                    "orange",
                )
            )
            continue
        return True
    return False


@task
def build(ctx, vstudio_root=None, arch="x64", major_version='7', debug=False):
    """
    Build the custom action library for the agent
    """

    if sys.platform != 'win32':
        print("Custom action library is only for Win32")
        raise Exit(code=1)

    package_version = get_version(ctx, url_safe=True, major_version=major_version)
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
        cmd = f'call "{vs_env_bat}" && msbuild {CUSTOM_ACTION_ROOT_DIR}\\cal /p:Configuration={configuration} /p:Platform={arch}'
    else:
        cmd = f'msbuild {CUSTOM_ACTION_ROOT_DIR}\\cal /p:Configuration={configuration} /p:Platform={arch}'

    cmd += verprops
    print(f"Build Command: {cmd}")

    # Try to run the command 3 times to alleviate transient
    # network failures
    succeeded = try_run(ctx, cmd, 3)
    if not succeeded:
        raise Exit("Failed to build the customaction.", code=1)

    artefacts = [
        {"source": "customaction.dll", "target": "customaction.dll"},
        {"source": "customaction.pdb", "target": f"customaction-{package_version}.pdb"},
        {"source": "customaction-tests.exe", "target": "customaction-tests.exe"},
    ]
    for artefact in artefacts:
        shutil.copy2(
            f"{CUSTOM_ACTION_ROOT_DIR}\\cal\\{arch}\\{configuration}\\{artefact['source']}",
            BIN_PATH + f"\\{artefact['target']}",
        )


@task
def clean(_, arch="x64", debug=False):
    configuration = "Release"
    if debug:
        configuration = "Debug"

    shutil.rmtree(f"{CUSTOM_ACTION_ROOT_DIR}\\cal\\{arch}\\{configuration}", BIN_PATH)


@task
def package(
    ctx,
    vstudio_root=None,
    omnibus_base_dir="c:\\omnibus-ruby",
    arch="x64",
    major_version='7',
    debug=False,
    rebuild=False,
):
    if os.getenv("OMNIBUS_BASE_DIR"):
        omnibus_base_dir = os.getenv("OMNIBUS_BASE_DIR")
    if rebuild:
        clean(ctx, arch, debug)
    build(ctx, vstudio_root, arch, major_version, debug)
    for file in glob.glob(BIN_PATH + "\\customaction*"):
        shutil.copy2(
            file,
            f"{omnibus_base_dir}\\src\\datadog-agent\\src\\github.com\\DataDog\\datadog-agent\\bin\\agent\\{os.path.basename(file)}",
        )
    cmd = "omnibus\\resources\\agent\\msi\\localbuild\\rebuild.bat"
    res = ctx.run(cmd, warn=True)
    if res.exited is None or res.exited > 0:
        print(
            color_message(
                f"Failed to run \"{cmd}\"",
                "orange",
            )
        )
