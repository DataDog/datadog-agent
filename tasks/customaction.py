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
def build(ctx, vstudio_root=None, arch="x64", major_version='7', debug=False):
    """
    Build the custom action library for the agent
    """

    if sys.platform != 'win32':
        print("Custom action library is only for Win32")
        raise Exit(code=1)

    ver = get_version_numeric_only(ctx, major_version=major_version)
    build_maj, build_min, build_patch = ver.split(".")
    verprops = " /p:MAJ_VER={build_maj} /p:MIN_VER={build_min} /p:PATCH_VER={build_patch} ".format(
        build_maj=build_maj, build_min=build_min, build_patch=build_patch
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
        cmd = 'call \"{}\" && msbuild {}\\cal\\customaction.vcxproj /p:Configuration={} /p:Platform={}'.format(
            vs_env_bat, CUSTOM_ACTION_ROOT_DIR, configuration, arch
        )
    else:
        cmd = 'msbuild {}\\cal\\customaction.vcxproj /p:Configuration={} /p:Platform={}'.format(
            CUSTOM_ACTION_ROOT_DIR, configuration, arch
        )

    cmd += verprops
    print("Build Command: %s" % cmd)

    ctx.run(cmd)
    artefacts = [
        "customaction.dll",
        "customaction.pdb",
    ]
    for artefact in artefacts:
        if arch is not None and arch == "x86":
            shutil.copy2("{}\\cal\\{}\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration, artefact), BIN_PATH)
        else:
            shutil.copy2("{}\\cal\\x64\\{}\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration, artefact), BIN_PATH)


@task
def clean(_, arch="x64", debug=False):
    configuration = "Release"
    if debug:
        configuration = "Debug"

    if arch is not None and arch == "x86":
        srcdll = "{}\\cal\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    else:
        srcdll = "{}\\cal\\x64\\{}".format(CUSTOM_ACTION_ROOT_DIR, configuration)
    shutil.rmtree(srcdll, BIN_PATH)
