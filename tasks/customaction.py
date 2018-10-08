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
from .build_tags import get_build_tags, get_default_build_tags, LINUX_ONLY_TAGS, REDHAT_AND_DEBIAN_ONLY_TAGS, REDHAT_AND_DEBIAN_DIST
from .go import deps

# constants
BIN_PATH = os.path.join(".", "bin", "agent")
AGENT_TAG = "datadog/agent:master"

@task
def build(ctx, vstudio_root=None):
    """
    Build the custom action library for the agent
    """

    if sys.platform != 'win32':
        print("Custom action library is only for Win32")
        raise Exit(code=1)

    cmd = ""
    if not os.getenv("VCINSTALLDIR"):
        print("VC Not installed in environment; checking other locations")

        vsroot = vstudio_root or os.getenv('VSTUDIO_ROOT')
        if not vsroot:
            print("Must have visual studio installed")
            raise Exit(code=2)
        vs_env_bat = '{}\\VC\\Auxiliary\\Build\\vcvars64.bat'.format(vsroot)
        cmd = 'call \"{}\" && msbuild omnibus\\resources\\agent\\msi\\cal\\customaction.vcxproj /p:Configuration=Release /p:Platform=x64'.format(vs_env_bat)
    else:
        cmd = 'msbuild omnibus\\resources\\agent\\msi\\cal\\customaction.vcxproj /p:Configuration=Release /p:Platform=x64'

    print("Build Command: %s" % cmd)

    ctx.run(cmd)

    shutil.copy2("omnibus/resources/agent/msi/cal/x64/release/customaction.dll", BIN_PATH)

